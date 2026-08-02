// Command goalign-analyzer is a vet-compatible multichecker for GoAlign.
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	goalign "github.com/gopherust-io/goalign/analysis"
)

func main() {
	singlechecker.Main(goalign.Analyzer)
}
