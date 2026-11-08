package source

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"docs-to-journeys/internal/common/proc"
)

func copyFile(_ context.Context, ref, out, _ string) error {
	b, err := os.ReadFile(ref)
	if err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o644)
}

// textutilConvert uses the macOS built-in converter for rich documents.
func textutilConvert(ctx context.Context, ref, out, _ string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("converting %s needs macOS textutil; export it to .md or .pdf instead", ref)
	}
	if _, err := os.Stat(ref); err != nil {
		return err
	}
	_, err := proc.Output(ctx, "textutil", "-convert", "txt", "-output", out, ref)
	return err
}

func fileResolvers() []resolver {
	return []resolver{
		extResolver{name: "text", exts: []string{".md", ".markdown", ".txt", ".feature"}, fetch: copyFile},
		extResolver{name: "rich", exts: []string{".docx", ".doc", ".rtf", ".rtfd", ".html", ".htm", ".odt", ".webarchive"}, fetch: textutilConvert},
	}
}
