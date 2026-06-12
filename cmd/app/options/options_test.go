package options

import (
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestNormalizeHomeResolvesRelativePath(t *testing.T) {
	t.Parallel()

	opts := NewOptions("HAPPLADYSAUCECLI")
	opts.Home = ".HAPPLADYSAUCECLI"

	if err := opts.NormalizeHome(); err != nil {
		t.Fatalf("NormalizeHome() error = %v", err)
	}
	if filepath.Base(opts.Home) != ".HAPPLADYSAUCECLI" {
		t.Fatalf("NormalizeHome() home = %q, want .HAPPLADYSAUCECLI base", opts.Home)
	}
	if !filepath.IsAbs(opts.Home) {
		t.Fatalf("NormalizeHome() home = %q, want absolute path", opts.Home)
	}
}

func TestAddFlagsRegistersHomeFlag(t *testing.T) {
	t.Parallel()

	opts := NewOptions("HAPPLADYSAUCECLI")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts.AddFlags(fs)

	if flag := fs.Lookup("home"); flag == nil {
		t.Fatal("home flag is not registered")
	}
}
