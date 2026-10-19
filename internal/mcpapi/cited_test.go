package mcpapi

import (
	"reflect"
	"testing"
)

func TestCitedAssets(t *testing.T) {
	doc := "Mira ![diagrama](assets/flujo.png) y [el contrato](assets/contrato.pdf).\n" +
		"Otra vez ![](assets/flujo.png). Esto no: assets/notas.md ni docs/x.png."
	got := citedAssets(doc)
	want := []string{"assets/flujo.png", "assets/contrato.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
