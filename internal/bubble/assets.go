package bubble

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// Images.
//
// They live in one place — `assets/` at the workspace root — and are referenced
// from anywhere as `assets/<name>`. Beside the page that uses them would mean a
// thread writing `../docs/…` and a nested page `../../…`, and a path that depends
// on where the writer happens to be standing is a path people get wrong.
//
// They are committed like everything else, which is the honest consequence of git
// being the content history: a repository that holds pictures grows and never
// shrinks. That is why there is a size limit and why it is small.

// maxAsset is the per-file ceiling. Deliberately modest: this is for the diagram
// in a page, not for a video. `BUBBLE_MAX_ASSET` moves it for somebody who has
// decided they want a bigger repository.
const maxAsset = 5 << 20 // 5 MiB

func assetLimit() int64 {
	if v := envInt("BUBBLE_MAX_ASSET"); v > 0 {
		return v
	}
	return maxAsset
}

func registerAssets(app core.App, t *tree.Tree) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Upload. Multipart because that is what a browser and a curl both speak
		// without anybody base64-encoding a picture into JSON.
		se.Router.POST("/api/workspaces/{id}/asset", func(e *core.RequestEvent) error {
			ws, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			limit := assetLimit()
			// Capped BEFORE reading: the point of a limit is to not have the bytes.
			e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, limit+1024)
			if err := e.Request.ParseMultipartForm(limit + 1024); err != nil {
				return e.BadRequestError(fmt.Sprintf("could not read the upload (limit %d bytes)", limit), err)
			}
			file, header, err := e.Request.FormFile("file")
			if err != nil {
				return e.BadRequestError("send the image as `file`", err)
			}
			defer file.Close()

			name := strings.TrimSpace(e.Request.FormValue("name"))
			if name == "" {
				name = header.Filename
			}
			docPath := path.Join(tree.DirAssets, slugifyFile(name))
			if !tree.IsAttachment(docPath) {
				return e.BadRequestError(
					"assets/ holds images (png, jpg, gif, webp, svg, avif) "+
						"and attached documents (pdf, txt, csv, json)", nil)
			}

			body, err := io.ReadAll(io.LimitReader(file, limit+1))
			if err != nil {
				return e.BadRequestError("could not read the upload", err)
			}
			if int64(len(body)) > limit {
				return e.BadRequestError(
					fmt.Sprintf("that file is larger than %d bytes", limit), nil)
			}

			docPath, err = placeAsset(t, repo, docPath, body, actorLabel(e.Auth))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			// An image is not production. It is a picture somebody attached; the
			// writing that uses it is what warms anything.
			return e.JSON(http.StatusOK, map[string]any{
				"workspace": ws.Id,
				"path":      docPath,
				"bytes":     len(body),
				// Para que quien lo insertó sepa si escribe `![]()` —una imagen
				// se ve— o `[]()` —un documento se abre—. Es lo único que
				// distingue a los dos en el markdown, y adivinarlo por la
				// extensión en el cliente sería una segunda lista que mantener.
				"image": tree.IsImage(docPath),
				"url":   fmt.Sprintf("/api/workspaces/%s/file?path=%s", ws.Id, docPath),
			})
		}).Bind(apis.RequireAuth())

		// Serving. Same boundary as everything else: a workspace's pictures are as
		// private as its writing.
		se.Router.GET("/api/workspaces/{id}/file", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			p := e.Request.URL.Query().Get("path")
			if !tree.IsAttachment(p) {
				return e.BadRequestError("this route serves what lives in assets/", nil)
			}
			body, err := t.ReadBytes(repo, p)
			if err != nil {
				return e.NotFoundError("", err)
			}

			ct := mime.TypeByExtension(strings.ToLower(path.Ext(p)))
			if ct == "" {
				ct = "application/octet-stream"
			}
			h := e.Response.Header()
			h.Set("Content-Type", ct)
			h.Set("Content-Length", strconv.Itoa(len(body)))
			// SVG is markup and can carry script. Served under a policy that permits
			// nothing and with nosniff, so neither an <img> nor somebody opening the
			// URL directly can run anything out of it.
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
			h.Set("X-Content-Type-Options", "nosniff")
			// Un adjunto se abre con el nombre con el que se subió, no con
			// `file?path=…`, que es lo que el navegador usaría si no se lo
			// decimos. `inline` y no `attachment`: un PDF se mira antes de
			// guardarse, y quien lo quiera guardar ya tiene el botón.
			if !tree.IsImage(p) {
				h.Set("Content-Disposition",
					fmt.Sprintf("inline; filename=%q", path.Base(p)))
			}
			h.Set("Cache-Control", "private, max-age=300")
			_, werr := e.Response.Write(body)
			return werr
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
}

// placing serializes choosing a name and writing it, so two uploads racing for
// `image.png` cannot both decide it is free.
var placing sync.Mutex

// placeAsset writes an asset under a name nobody else is using, and returns it.
//
// A path in assets/ is never rewritten with different bytes. Every screenshot
// pasted from the clipboard arrives as `image.png`: writing it over the last one
// replaced the picture in every document that already cited it, and the client —
// which caches an image by its path for the life of the tab — kept drawing the
// old bytes under the new paste. With a path that never changes what it holds,
// that cache is correct.
//
// The same bytes under the same name are the same picture, and reuse the path
// instead of making a copy. Different bytes take the first free `stem-N.ext`.
func placeAsset(t *tree.Tree, repo, docPath string, body []byte, actor string) (string, error) {
	placing.Lock()
	defer placing.Unlock()

	ext := path.Ext(docPath)
	stem := strings.TrimSuffix(docPath, ext)
	for n := 1; ; n++ {
		candidate := docPath
		if n > 1 {
			candidate = stem + "-" + strconv.Itoa(n) + ext
		}
		existing, err := t.ReadBytes(repo, candidate)
		if err == nil {
			if bytes.Equal(existing, body) {
				return candidate, nil
			}
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if err := t.WriteBytes(repo, candidate, body, actor, "asset: "+candidate); err != nil {
			return "", err
		}
		return candidate, nil
	}
}

// slugifyFile keeps a readable name and drops anything that would make it a path.
func slugifyFile(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := strings.ToLower(path.Ext(name))
	stem := slugify(strings.TrimSuffix(name, path.Ext(name)))
	if stem == "" {
		stem = "image"
	}
	return stem + ext
}

func envInt(key string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(getenv(key)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
