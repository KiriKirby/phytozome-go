# PHgo motif checker

The BLAST external-reference dialog contains three independent reference sections: UniProt, InterPro, and the optional local Motif checker. The Motif checker is disabled by default. InterPro's existing parameter controls remain visible so old settings and snapshots remain understandable, but the former conserved-region status calculation is retained only for compatibility and is not executed in new runs; its generated status column is not added to new result tables.

Enter motifs separated by `,` or `，`. Matching is case-insensitive and searches the complete hit protein sequence. Literal residues must match exactly; `X` matches one non-gap residue; `-`/`－`/`−` match one gap; `[A/B]` and `(A/B)` (including full-width forms) allow alternatives. Slashless `[ABC]` means one of A, B, or C.

`phgo_motif_matches` is a 1-based comma-separated list of motifs that matched completely anywhere in the hit sequence. For example, `1,3` means motifs 1 and 3 matched. No match is blank. A cell is green only when every configured motif matched; partial matches keep the normal text color. The original motif string is stored in `.pgo` external-reference settings and the XLSX `Query Metadata` sheet as `phgo_motif_patterns`.

The checker is a direct sequence-pattern test. It does not query InterPro, infer function, tolerate substitutions outside explicit alternatives, or treat a partial motif as a match.
