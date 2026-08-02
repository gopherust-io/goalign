// Package pkgscan resolves imported type sizes via go/packages (opt-in --packages).
package pkgscan

import (
	"fmt"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/gopherust-io/goalign/internal/layout"
)

// LoadTypeSizes loads packages under patterns and returns a name→Info map for
// defined named types (package-qualified selectors and import path keys).
// Dependency packages are included via packages.Visit so imported types resolve.
func LoadTypeSizes(patterns []string, goarch string) (map[string]layout.Info, error) {
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedDeps | packages.NeedImports,
		Env:  appendEnviron(goarch),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("packages load had errors")
	}

	out := make(map[string]layout.Info)
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg.Types == nil || pkg.TypesSizes == nil {
			return
		}
		sizes := pkg.TypesSizes
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || obj.Type() == nil {
				continue
			}
			info := infoOf(sizes, obj.Type())
			if info.IsUnknown() {
				continue
			}
			// Prefer package-qualified keys to avoid cross-package bare-name clashes.
			if pkg.Name != "" {
				key := pkg.Name + "." + name
				if _, exists := out[key]; !exists {
					out[key] = info
				}
			}
			if pkg.PkgPath != "" {
				key := pkg.PkgPath + "." + name
				if _, exists := out[key]; !exists {
					out[key] = info
				}
			}
			// Bare name only for types defined in the initially matched packages.
			if isRootPkg(pkgs, pkg) {
				if _, exists := out[name]; !exists {
					out[name] = info
				}
			}
		}
	})
	return out, nil
}

func isRootPkg(roots []*packages.Package, pkg *packages.Package) bool {
	for _, r := range roots {
		if r == pkg {
			return true
		}
	}
	return false
}

func infoOf(sizes types.Sizes, t types.Type) (info layout.Info) {
	if t == nil {
		return layout.Unknown
	}
	// go/types can panic on type parameters / incomplete generic types in deps.
	defer func() {
		if recover() != nil {
			info = layout.Unknown
		}
	}()
	if _, ok := t.(*types.TypeParam); ok {
		return layout.Unknown
	}
	if named, ok := t.(*types.Named); ok && named.TypeParams().Len() > 0 && named.TypeArgs().Len() == 0 {
		return layout.Unknown
	}
	size := sizes.Sizeof(t)
	align := sizes.Alignof(t)
	if size < 0 || align < 1 {
		return layout.Unknown
	}
	return layout.Info{Size: int(size), Align: int(align)}
}

func appendEnviron(goarch string) []string {
	if goarch == "" {
		return nil
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, "GOARCH=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "GOARCH="+goarch)
}
