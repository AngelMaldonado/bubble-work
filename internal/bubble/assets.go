package bubble

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

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
			if !tree.IsImage(docPath) {
				return e.BadRequestError(
					"only images live in assets/ — png, jpg, gif, webp, svg, avif", nil)
			}

			body, err := io.ReadAll(io.LimitReader(file, limit+1))
			if err != nil {
				return e.BadRequestError("could not read the upload", err)
			}
			if int64(len(body)) > limit {
				return e.BadRequestError(
					fmt.Sprintf("that image is larger than %d bytes", limit), nil)
			}

			if err := t.WriteBytes(repo, docPath, body, actorLabel(e.Auth), "asset: "+docPath); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			// An image is not production. It is a picture somebody attached; the
			// writing that uses it is what warms anything.
			return e.JSON(http.StatusOK, map[string]any{
				"workspace": ws.Id,
				"path":      docPath,
				"bytes":     len(body),
				"url":       fmt.Sprintf("/api/workspaces/%s/file?path=%s", ws.Id, docPath),
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
			if !tree.IsImage(p) {
				return e.BadRequestError("this route serves images", nil)
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
			h.Set("Cache-Control", "private, max-age=300")
			_, werr := e.Response.Write(body)
			return werr
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
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
