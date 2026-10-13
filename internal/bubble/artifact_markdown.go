package bubble

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Each publication is a full immutable revision. The visible document is the
// latest one, even when artifact_read(since) omits its entry from the tail.
type ArtifactDocument struct {
	Filename string `json:"filename"`
	Markdown string `json:"markdown,omitempty"`
	Seq      int    `json:"seq"`
	HTML     string `json:"html,omitempty"`
}

func latestDocument(entries []Entry) *ArtifactDocument {
	for i := len(entries) - 1; i >= 0; i-- {
		if e := entries[i]; e.Type == "document" {
			return &ArtifactDocument{Filename: str(e.Data, "filename"), Markdown: str(e.Data, "markdown"), Seq: e.Seq}
		}
	}
	return nil
}

func validateMarkdown(e Entry, prior []Entry) error {
	if e.Type != "document" {
		return fmt.Errorf("a markdown artifact takes document publications, not %q", e.Type)
	}
	name := str(e.Data, "filename")
	if name == "" || len([]rune(name)) > 120 || !strings.HasSuffix(strings.ToLower(name), ".md") || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 || name != strings.TrimSpace(name) || strings.EqualFold(name, ".md") {
		return fmt.Errorf("filename must be a plain .md name, not a path")
	}
	text, ok := e.Data["markdown"].(string)
	if !ok || len(text) > 128<<10 {
		return fmt.Errorf("markdown must be a string of at most 128 KiB")
	}
	raw, ok := e.Data["base"]
	if !ok || raw == nil {
		return fmt.Errorf("base is required: 0 for the first publication, otherwise the document's seq")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var base int
	if err := json.Unmarshal(encoded, &base); err != nil || base < 0 {
		return fmt.Errorf("base must be a nonnegative integer seq")
	}
	expected := 0
	if doc := latestDocument(prior); doc != nil {
		expected = doc.Seq
	}
	if base != expected {
		return fmt.Errorf("document changed: base %d, current seq %d; read before publishing again", base, expected)
	}
	return nil
}
