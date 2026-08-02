#!/usr/bin/env bash
# Competitive wall-clock analyze on the shared density corpus.
# GoAlign uses AST heuristics; betteralign/fieldalignment use go/packages.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

CORPUS="${CORPUS:-testdata/benchcorpus/density}"
REPEATS="${REPEATS:-5}"
BETTERALIGN_VER="${BETTERALIGN_VER:-v0.14.3}"
FIELDALIGNMENT_VER="${FIELDALIGNMENT_VER:-v0.42.0}"
OUT="${OUT:-artifacts/benchcmp.md}"

mkdir -p "$(dirname "$OUT")" bin

export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"

echo "== building goalign =="
go build -o bin/goalign .

echo "== installing competitors =="
GOPRIVATE= go install "github.com/dkorunic/betteralign/cmd/betteralign@${BETTERALIGN_VER}"
GOPRIVATE= go install "golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@${FIELDALIGNMENT_VER}"

GOBIN="$(go env GOPATH)/bin"
GOALIGN_BIN="$root/bin/goalign"
BETTER_BIN="$GOBIN/betteralign"
FIELD_BIN="$GOBIN/fieldalignment"

for bin in "$GOALIGN_BIN" "$BETTER_BIN" "$FIELD_BIN"; do
	if [[ ! -x "$bin" ]]; then
		echo "missing binary: $bin" >&2
		exit 1
	fi
done

mean_ms() {
	local label="$1"
	shift
	python3 - "$REPEATS" "$label" "$@" <<'PY'
import subprocess, sys, time
repeats = int(sys.argv[1])
label = sys.argv[2]
cmd = sys.argv[3:]
# warm once
subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
samples = []
for _ in range(repeats):
    t0 = time.perf_counter()
    subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    samples.append((time.perf_counter() - t0) * 1000)
samples.sort()
mean = sum(samples) / len(samples)
med = samples[len(samples)//2]
print(f"{label}\t{mean:.1f}\t{med:.1f}", flush=True)
PY
}

echo "== warming + timing (${REPEATS} repeats) =="
goalign_line=$(mean_ms goalign "$GOALIGN_BIN" analyze -r --arch=amd64 -f json "./${CORPUS}")
better_line=$(mean_ms betteralign "$BETTER_BIN" "./${CORPUS}")
field_line=$(mean_ms fieldalignment "$FIELD_BIN" "./${CORPUS}")

goalign_ms=$(echo "$goalign_line" | cut -f2)
better_ms=$(echo "$better_line" | cut -f2)
field_ms=$(echo "$field_line" | cut -f2)
goalign_med=$(echo "$goalign_line" | cut -f3)
better_med=$(echo "$better_line" | cut -f3)
field_med=$(echo "$field_line" | cut -f3)

{
	echo "# Competitive analyze (wall-clock)"
	echo
	echo "- Corpus: \`${CORPUS}\`"
	echo "- Repeats: ${REPEATS} (mean / median ms)"
	echo "- $(go version)"
	echo "- $(uname -s)/$(uname -m) — $(sysctl -n machdep.cpu.brand_string 2>/dev/null || true)"
	echo
	echo "| Tool | mean ms | median ms |"
	echo "|------|--------:|----------:|"
	echo "| goalign analyze -r -f json | ${goalign_ms} | ${goalign_med} |"
	echo "| betteralign | ${better_ms} | ${better_med} |"
	echo "| fieldalignment | ${field_ms} | ${field_med} |"
	echo
	echo "Preface: goalign is AST-only (no typechecker). betteralign/fieldalignment load packages via \`go/packages\` and optimize for true type sizes + GC ptrdata. Numbers are directional."
} | tee "$OUT"

echo "Wrote $OUT"
