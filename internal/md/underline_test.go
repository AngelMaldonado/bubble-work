package md

import (
	"strings"
	"testing"
)

func TestUnderlineIsLimitedInlineMarkup(t *testing.T) {
	for _, tc := range []struct{ in, want, not string }{
		{"Texto ++subrayado++ y **negrita**", "<u>subrayado</u>", ""},
		{"a ++b++ c ++d++", "<u>d</u>", ""},
		{"1 + 1 ++ 2", "1 + 1 ++ 2", "<u>"},
		{"`++code++`", "<code>++code++</code>", "<u>"},
		{"++<script>x</script>++", "<u>", "<script>"},
		{"c+++ uno", "c+++ uno", "<u>"},
	} {
		got := RenderHTML(tc.in)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q: %q lacks %q", tc.in, got, tc.want)
		}
		if tc.not != "" && strings.Contains(got, tc.not) {
			t.Errorf("%q: %q contains %q", tc.in, got, tc.not)
		}
	}
}
