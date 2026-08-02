// Package diff produces unified diffs for goalign fix --diff.
package diff

import (
	"fmt"
	"strings"
)

// Unified returns a unified diff between old and new content for filename.
// Empty string means no changes.
func Unified(filename string, old, new []byte) string {
	if string(old) == string(new) {
		return ""
	}
	a := splitLines(string(old))
	b := splitLines(string(new))
	chunks := myers(a, b)
	if len(chunks) == 0 {
		return ""
	}

	var bld strings.Builder
	fmt.Fprintf(&bld, "--- a/%s\n+++ b/%s\n", filename, filename)
	for _, c := range chunks {
		fmt.Fprintf(&bld, "@@ -%d,%d +%d,%d @@\n", c.aStart+1, c.aCount, c.bStart+1, c.bCount)
		for _, line := range c.lines {
			bld.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				bld.WriteByte('\n')
			}
		}
	}
	return bld.String()
}

type chunk struct {
	lines  []string
	aStart int
	aCount int
	bStart int
	bCount int
}

type edit struct {
	tag byte // ' ', '-', '+'
	a   int
	b   int
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// myers is a simple O(ND) LCS-based edit script with context hunks.
func myers(a, b []string) []chunk {
	n, m := len(a), len(b)
	// DP LCS lengths
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var edits []edit
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			edits = append(edits, edit{tag: ' ', a: i, b: j})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			edits = append(edits, edit{tag: '-', a: i, b: j})
			i++
		} else {
			edits = append(edits, edit{tag: '+', a: i, b: j})
			j++
		}
	}
	for i < n {
		edits = append(edits, edit{tag: '-', a: i, b: j})
		i++
	}
	for j < m {
		edits = append(edits, edit{tag: '+', a: i, b: j})
		j++
	}

	return groupHunks(a, b, edits, 3)
}

func groupHunks(a, b []string, edits []edit, ctx int) []chunk {
	changed := make([]bool, len(edits))
	for i, e := range edits {
		changed[i] = e.tag != ' '
	}
	var chunks []chunk
	i := 0
	for i < len(edits) {
		for i < len(edits) && !changed[i] {
			i++
		}
		if i >= len(edits) {
			break
		}
		start := i - ctx
		if start < 0 {
			start = 0
		}
		end := i + 1
		for end < len(edits) {
			if changed[end] {
				end++
				continue
			}
			// look ahead for next change within 2*ctx
			next := end
			for next < len(edits) && !changed[next] {
				next++
			}
			if next >= len(edits) {
				break
			}
			if next-end <= 2*ctx {
				end = next + 1
				continue
			}
			break
		}
		endCtx := end + ctx
		if endCtx > len(edits) {
			endCtx = len(edits)
		}

		c := chunk{}
		aStart, bStart := -1, -1
		for k := start; k < endCtx; k++ {
			e := edits[k]
			if aStart < 0 && (e.tag == ' ' || e.tag == '-') {
				aStart = e.a
			}
			if bStart < 0 && (e.tag == ' ' || e.tag == '+') {
				bStart = e.b
			}
			switch e.tag {
			case ' ':
				c.lines = append(c.lines, " "+a[e.a])
				c.aCount++
				c.bCount++
			case '-':
				c.lines = append(c.lines, "-"+a[e.a])
				c.aCount++
			case '+':
				c.lines = append(c.lines, "+"+b[e.b])
				c.bCount++
			}
		}
		if aStart < 0 {
			aStart = 0
		}
		if bStart < 0 {
			bStart = 0
		}
		c.aStart = aStart
		c.bStart = bStart
		chunks = append(chunks, c)
		i = endCtx
	}
	return chunks
}
