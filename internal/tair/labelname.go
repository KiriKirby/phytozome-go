package tair

import (
	"context"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
)

var tairPhytozomeSpeciesOnce sync.Once
var tairPhytozomeSpecies model.SpeciesCandidate
var tairPhytozomeSpeciesOK bool

func tairReferencePhytozomeSpecies() (model.SpeciesCandidate, bool) {
	tairPhytozomeSpeciesOnce.Do(func() {
		client := phytozome.NewClient(nil)
		candidates, err := client.FetchSpeciesCandidates(context.Background())
		if err != nil {
			return
		}
		for _, candidate := range candidates {
			if candidate.ProteomeID == 167 || strings.EqualFold(strings.TrimSpace(candidate.JBrowseName), "Athaliana_TAIR10") {
				tairPhytozomeSpecies = candidate
				tairPhytozomeSpeciesOK = true
				return
			}
		}
	})
	return tairPhytozomeSpecies, tairPhytozomeSpeciesOK
}

func (c *Client) ResolveTAIRKeywordRowLabelCandidates(ctx context.Context, row model.KeywordResultRow) ([]string, string) {
	if aliases, sourceType := c.resolveTAIRViaPhytozomeRow(ctx, row); len(aliases) > 0 {
		return aliases, sourceType
	}
	if aliases := tairOtherNamesFallbackAliases(row); len(aliases) > 0 {
		return aliases, "tair other_names"
	}
	return nil, ""
}

func (c *Client) ResolveTAIRFamilyCandidateLabelCandidates(ctx context.Context, version model.SpeciesCandidate, candidate model.SpeciesCandidate) ([]string, string) {
	familyKey := strings.TrimSpace(firstNonEmpty(candidate.GroupKey, candidate.JBrowseName))
	if familyKey == "" {
		return nil, ""
	}
	rows, err := c.SearchFamilyKeywordRows(ctx, version, familyKey)
	if err != nil || len(rows) == 0 {
		return nil, ""
	}
	aliases := make([]string, 0, len(rows)*3)
	sourceType := ""
	for _, row := range rows {
		rowAliases, rowSource := c.ResolveTAIRKeywordRowLabelCandidates(ctx, row)
		if sourceType == "" && strings.TrimSpace(rowSource) != "" {
			sourceType = strings.TrimSpace(rowSource)
		}
		aliases = append(aliases, rowAliases...)
	}
	if aliases = uniqueStrings(aliases); len(aliases) > 0 {
		return aliases, sourceType
	}
	return nil, ""
}

func (c *Client) resolveTAIRViaPhytozomeRow(ctx context.Context, row model.KeywordResultRow) ([]string, string) {
	species, ok := tairReferencePhytozomeSpecies()
	if !ok {
		return nil, ""
	}
	terms := tairPhytozomeSearchTerms(row)
	if len(terms) == 0 {
		return nil, ""
	}
	lookup := phytozome.NewClient(c.httpClient)
	for _, term := range terms {
		rows, err := lookup.SearchKeywordRows(ctx, species, term)
		if err != nil {
			continue
		}
		if aliases, sourceType := tairPhytozomeAliasCandidates(rows); len(aliases) > 0 {
			return aliases, sourceType
		}
	}
	return nil, ""
}

func tairPhytozomeSearchTerms(row model.KeywordResultRow) []string {
	terms := make([]string, 0, 8)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			terms = append(terms, value)
		}
	}
	add(row.ProteinID)
	add(row.SequenceID)
	add(row.TranscriptID)
	add(row.GeneIdentifier)
	add(stripTranscriptSuffix(firstNonEmpty(row.TranscriptID, row.SequenceID, row.ProteinID, row.GeneIdentifier)))
	return uniqueStrings(terms)
}

func tairPhytozomeAliasCandidates(rows []model.KeywordResultRow) ([]string, string) {
	synonyms := make([]string, 0, len(rows)*2)
	symbols := make([]string, 0, len(rows)*2)
	autoDefine := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		synonyms = append(synonyms, labelname.SplitAliases(row.Synonyms)...)
		symbols = append(symbols, labelname.SplitAliases(row.Symbols)...)
		autoDefine = append(autoDefine, labelname.AutoDefineCandidates(row.AutoDefine)...)
	}
	if synonyms = uniqueStrings(synonyms); len(synonyms) > 0 {
		return synonyms, "phytozome synonyms"
	}
	if symbols = uniqueStrings(symbols); len(symbols) > 0 {
		return symbols, "phytozome symbols"
	}
	if autoDefine = uniqueStrings(autoDefine); len(autoDefine) > 0 {
		return autoDefine, "phytozome auto_define"
	}
	return nil, ""
}

func tairOtherNamesFallbackAliases(row model.KeywordResultRow) []string {
	return uniqueStrings(labelname.SplitAliases(row.Synonyms))
}
