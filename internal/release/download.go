package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Asset es cómo se llama el binario de una plataforma en un release.
func Asset(goos, goarch string) string { return "bubble-" + goos + "-" + goarch }

// Download baja el binario de una versión y lo deja en `dst`, sólo si sus bytes
// son los que el release dice.
//
// Lo que esto comprueba es INTEGRIDAD, no procedencia: `SHA256SUMS` viaja en el
// mismo release que el binario, así que quien pudiera publicar un release falso
// publicaría también sus sumas. Detecta una descarga a medias, un proxy que
// mete basura y un espejo desactualizado — y no detecta un release hostil.
// Firmarlo es lo que convertiría esto en confianza, y no está hecho.
func Download(ctx context.Context, cl *http.Client, tag, goos, goarch, dst string) error {
	name := Asset(goos, goarch)
	sums, err := get(ctx, cl, url(tag, "SHA256SUMS"))
	if err != nil {
		return fmt.Errorf("no pude leer SHA256SUMS de %s: %w", tag, err)
	}
	want := sumFor(string(sums), name)
	if want == "" {
		return fmt.Errorf("%s no publica %s: esta plataforma no tiene binario en esa versión", tag, name)
	}

	res, err := do(ctx, cl, url(tag, name))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// A un temporal y luego renombrar: un binario a medias con el nombre bueno
	// es una instancia que no arranca.
	tmp := dst + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), res.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(tmp)
		return fmt.Errorf("los bytes de %s no son los que dice el release (%s ≠ %s)", name, got[:12], want[:12])
	}
	return os.Rename(tmp, dst)
}

func url(tag, asset string) string {
	return "https://github.com/" + Repo + "/releases/download/" + tag + "/" + asset
}

func do(ctx context.Context, cl *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	res, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("%s respondió %d", u, res.StatusCode)
	}
	return res, nil
}

func get(ctx context.Context, cl *http.Client, u string) ([]byte, error) {
	res, err := do(ctx, cl, u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(io.LimitReader(res.Body, 1<<20))
}

// sumFor saca de un SHA256SUMS la suma de un archivo. El formato es el de
// `sha256sum`: «<suma>  <nombre>», con dos espacios.
func sumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			return f[0]
		}
	}
	return ""
}
