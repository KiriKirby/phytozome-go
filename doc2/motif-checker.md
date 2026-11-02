# PHgo motif checker

The BLAST external-reference dialog provides an optional local motif checker. It is disabled by default. The former InterPro conserved-region status evaluator remains in source for backward compatibility but is not executed and its status controls/column are not shown.

Enter motifs separated by `,` or `，`. Matching is case-insensitive and searches the complete hit protein sequence. Literal residues must match exactly; `X` matches one non-gap residue; `-`/`－`/`−` match one gap; `[A/B]` and `(A/B)` (including full-width forms) allow alternatives. Slashless `[ABC]` means one of A, B, or C.

`phgo_motif_matches` is a 1-based comma-separated list of motifs that matched completely anywhere in the hit sequence. For example, `1,3` means motifs 1 and 3 matched. No match is blank. A cell is green only when every configured motif matched; partial matches keep the normal text color. The original motif string is stored in `.pgo` external-reference settings and the XLSX `Query Metadata` sheet as `phgo_motif_patterns`.

The checker is a direct sequence-pattern test. It does not query InterPro, infer function, tolerate substitutions outside explicit alternatives, or treat a partial motif as a match.
