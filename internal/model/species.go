// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package model

import "strings"

// SpeciesCandidate is the first-step selection unit for the BLAST workflow.
type SpeciesCandidate struct {
	ProteomeID  int
	JBrowseName string
	GenomeLabel string
	CommonName  string
	ReleaseDate string
	SearchAlias string
	IsOfficial  bool
}

func (s SpeciesCandidate) DisplayLabel() string {
	label := s.GenomeLabel
	if s.SearchAlias != "" {
		label = s.SearchAlias
	}

	parts := []string{label}
	if s.CommonName != "" {
		parts = append(parts, "("+s.CommonName+")")
	}
	if s.ReleaseDate != "" {
		parts = append(parts, "["+s.ReleaseDate+"]")
	}
	if s.IsOfficial {
		// prepend an official tag so callers can show it prominently
		parts = append([]string{"[official]"}, parts...)
	}
	return strings.Join(parts, " ")
}

func (s SpeciesCandidate) SearchText() string {
	return strings.ToLower(strings.Join([]string{
		s.JBrowseName,
		s.GenomeLabel,
		s.CommonName,
		s.SearchAlias,
	}, " "))
}
