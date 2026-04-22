package model

import "strings"

// SpeciesCandidate is the first-step selection unit for the BLAST workflow.
type SpeciesCandidate struct {
	ProteomeID  int
	JBrowseName string
	GenomeLabel string
	CommonName  string
	ReleaseDate string
}

func (s SpeciesCandidate) DisplayLabel() string {
	parts := []string{s.GenomeLabel}
	if s.CommonName != "" {
		parts = append(parts, "("+s.CommonName+")")
	}
	if s.ReleaseDate != "" {
		parts = append(parts, "["+s.ReleaseDate+"]")
	}
	return strings.Join(parts, " ")
}

func (s SpeciesCandidate) SearchText() string {
	return strings.ToLower(strings.Join([]string{
		s.JBrowseName,
		s.GenomeLabel,
		s.CommonName,
	}, " "))
}
