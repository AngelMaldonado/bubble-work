package bubble

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y += 7 {
		for x := 0; x < w; x += 7 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestForModelKeepsSmallImages(t *testing.T) {
	src := pngOf(800, 600)
	out, mt, ok := ForModel(Attachment{MIME: "image/png", Bytes: src})
	if !ok || mt != "image/png" || !bytes.Equal(out, src) {
		t.Fatalf("una imagen chica va tal cual: ok=%v mt=%s same=%v", ok, mt, bytes.Equal(out, src))
	}
}

func TestForModelReducesLargeImages(t *testing.T) {
	out, mt, ok := ForModel(Attachment{MIME: "image/png", Bytes: pngOf(4000, 1000)})
	if !ok {
		t.Fatal("no se pudo reducir")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != maxEdge || cfg.Height != 500 {
		t.Fatalf("se reduce a %d de lado y conserva la proporción: %dx%d (%s)", maxEdge, cfg.Width, cfg.Height, mt)
	}
}

func TestForModelJPEGStaysJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3000, 3000))
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	_, mt, ok := ForModel(Attachment{MIME: "image/jpeg", Bytes: b.Bytes()})
	if !ok || mt != "image/jpeg" {
		t.Fatalf("una foto sigue siendo JPEG: ok=%v mt=%s", ok, mt)
	}
}

func TestForModelRefusesWhatItCannotDecode(t *testing.T) {
	if _, _, ok := ForModel(Attachment{MIME: "image/avif", Bytes: []byte("x")}); ok {
		t.Fatal("AVIF no se decodifica: queda la descarga")
	}
	if _, _, ok := ForModel(Attachment{MIME: "application/pdf", Bytes: []byte("%PDF")}); ok {
		t.Fatal("un PDF no es una imagen")
	}
}

func TestLinksAreSingleUse(t *testing.T) {
	key := newLink(oneLink{user: "u", expires: nowPlus(), get: &AttachRef{Space: "s"}})
	if _, ok := takeLink(key); !ok {
		t.Fatal("el enlace vale la primera vez")
	}
	if _, ok := takeLink(key); ok {
		t.Fatal("y no la segunda")
	}
	old := newLink(oneLink{user: "u", expires: nowPlus().Add(-2 * linkTTL)})
	if _, ok := takeLink(old); ok {
		t.Fatal("uno vencido no vale")
	}
}

func nowPlus() time.Time { return time.Now().Add(linkTTL) }
