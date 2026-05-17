package tair

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/model"
)

type localBlastThreadsContextKey struct{}

func WithLocalBlastThreads(ctx context.Context, threads int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if threads < 1 {
		threads = 1
	}
	return context.WithValue(ctx, localBlastThreadsContextKey{}, threads)
}

func (c *Client) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	program := normalizeProgram(req.Program)
	req.Program = strings.ToUpper(program)
	return c.RunLocalBlast(ctx, req)
}

func (c *Client) RunLocalBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	if strings.TrimSpace(req.Sequence) == "" {
		return model.BlastJob{}, fmt.Errorf("empty BLAST query sequence")
	}
	rel, err := c.releaseForVersion(req.Species)
	if err != nil {
		return model.BlastJob{}, err
	}
	program := normalizeProgram(req.Program)
	dbURL, dbType, err := localBlastDB(rel, program)
	if err != nil {
		return model.BlastJob{}, err
	}
	if err := blastplus.EnsureToolsOnPath("makeblastdb", program); err != nil {
		return model.BlastJob{}, err
	}
	dir, err := cacheDir("localblast", sanitizeFileName(rel.Name))
	if err != nil {
		return model.BlastJob{}, err
	}
	fastaPath, err := downloadToCache(ctx, c.httpClient, dbURL, dir)
	if err != nil {
		return model.BlastJob{}, err
	}
	dbPrefix := filepath.Join(dir, sanitizeFileName(strings.ToLower(program)+"_"+rel.Name)+"_db")
	if err := ensureBlastDB(ctx, fastaPath, dbPrefix, dbType); err != nil {
		return model.BlastJob{}, err
	}
	fastaIndex, err := buildFastaIndex(fastaPath)
	if err != nil {
		return model.BlastJob{}, err
	}
	result, err := runBlastAndParse(ctx, program, dbPrefix, fastaIndex, req)
	if err != nil {
		return model.BlastJob{}, err
	}
	enrichLocalBlastRows(ctx, c, rel, req.Species, &result.Rows)
	job := model.BlastJob{
		JobID:   fmt.Sprintf("tair-local-%s-%d", sanitizeFileName(rel.Name), time.Now().UnixNano()),
		Message: fmt.Sprintf("local %s completed", strings.ToUpper(program)),
	}
	result.JobID = job.JobID
	result.Message = job.Message
	c.mu.Lock()
	c.localResults[job.JobID] = result
	c.mu.Unlock()
	return job, nil
}

func (c *Client) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	_ = pollInterval
	_ = timeout
	select {
	case <-ctx.Done():
		return model.BlastResult{}, ctx.Err()
	default:
	}
	c.mu.RLock()
	result, ok := c.localResults[jobID]
	c.mu.RUnlock()
	if !ok {
		return model.BlastResult{}, fmt.Errorf("TAIR local BLAST result %s is not available", jobID)
	}
	return result, nil
}

func localBlastDB(rel releaseInfo, program string) (string, string, error) {
	switch program {
	case "blastp", "blastx":
		if rel.ProteinURL == "" {
			return "", "", fmt.Errorf("TAIR %s has no official protein FASTA in the current public download set", rel.Name)
		}
		return rel.ProteinURL, "prot", nil
	case "blastn", "tblastn":
		if rel.NucleotideURL == "" {
			return "", "", fmt.Errorf("TAIR %s has no official nucleotide FASTA in the current public download set", rel.Name)
		}
		return rel.NucleotideURL, "nucl", nil
	default:
		return "", "", fmt.Errorf("unsupported BLAST program %q", program)
	}
}

func normalizeProgram(value string) string {
	p := strings.ToLower(strings.TrimSpace(value))
	p = strings.TrimPrefix(p, "local:")
	switch {
	case strings.Contains(p, "tblastn"):
		return "tblastn"
	case strings.Contains(p, "blastx"):
		return "blastx"
	case strings.Contains(p, "blastp"):
		return "blastp"
	default:
		return "blastn"
	}
}

func ensureBlastDB(ctx context.Context, fastaPath string, dbPrefix string, dbType string) error {
	if blastDBComplete(dbPrefix, dbType) {
		return nil
	}
	_ = removeBlastDBFiles(dbPrefix)
	args := []string{"-in", fastaPath, "-dbtype", dbType, "-parse_seqids", "-blastdb_version", "4", "-out", dbPrefix}
	cmd := exec.CommandContext(ctx, "makeblastdb", args...)
	cmd.Env = append(os.Environ(), "BLASTDB_VERSION=4")
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = removeBlastDBFiles(dbPrefix)
		args = []string{"-in", fastaPath, "-dbtype", dbType, "-blastdb_version", "4", "-out", dbPrefix}
		cmd = exec.CommandContext(ctx, "makeblastdb", args...)
		cmd.Env = append(os.Environ(), "BLASTDB_VERSION=4")
		stderr.Reset()
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr
		if err2 := cmd.Run(); err2 != nil {
			return fmt.Errorf("makeblastdb failed: %w%s", err2, capturedOutput(stderr.String()))
		}
	}
	if !blastDBComplete(dbPrefix, dbType) {
		return fmt.Errorf("makeblastdb completed but DB files are missing for %s", dbPrefix)
	}
	return nil
}

func blastDBComplete(prefix string, dbType string) bool {
	var exts []string
	switch dbType {
	case "prot":
		exts = []string{".pin", ".phr", ".psq"}
	case "nucl":
		exts = []string{".nin", ".nhr", ".nsq"}
	default:
		return false
	}
	for _, ext := range exts {
		info, err := os.Stat(prefix + ext)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func removeBlastDBFiles(prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return nil
	}
	matches, _ := filepath.Glob(prefix + ".*")
	for _, match := range matches {
		_ = os.Remove(match)
	}
	return nil
}

func runBlastAndParse(ctx context.Context, program string, dbPrefix string, fastaIndex map[string]fastaEntry, req model.BlastRequest) (model.BlastResult, error) {
	queryFASTA, queryLengths, fallbackQueryLength, err := localBlastQueryFASTA(req.Sequence)
	if err != nil {
		return model.BlastResult{}, err
	}
	tmpDir, err := os.MkdirTemp("", "tair-blast-query-*")
	if err != nil {
		return model.BlastResult{}, err
	}
	defer os.RemoveAll(tmpDir)
	queryPath := filepath.Join(tmpDir, "query.fasta")
	if err := os.WriteFile(queryPath, []byte(queryFASTA), 0o644); err != nil {
		return model.BlastResult{}, err
	}
	outPath := filepath.Join(tmpDir, "blast.tsv")
	fields := []string{"qseqid", "sseqid", "pident", "length", "mismatch", "gapopen", "qstart", "qend", "sstart", "send", "evalue", "bitscore"}
	if program == "blastp" || program == "blastx" || program == "tblastn" {
		fields = append(fields, "positive")
	}
	if program == "blastx" || program == "tblastn" {
		fields = append(fields, "qframe", "sframe")
	}
	args := []string{"-query", queryPath, "-db", dbPrefix, "-outfmt", "6 " + strings.Join(fields, " "), "-out", outPath}
	if threads := localBlastThreads(ctx); threads > 1 {
		args = append(args, "-num_threads", strconv.Itoa(threads))
	}
	if evalue := strings.TrimSpace(req.EValue); evalue != "" && evalue != "-1" {
		args = append(args, "-evalue", evalue)
	}
	if req.AlignmentsToShow > 0 {
		args = append(args, "-max_target_seqs", strconv.Itoa(req.AlignmentsToShow))
	}
	if word := normalizedWordSize(req.WordLength); word != "" {
		args = append(args, "-word_size", word)
	}
	if !req.AllowGaps {
		args = append(args, "-ungapped")
		if program != "blastn" {
			args = append(args, "-comp_based_stats", "F")
		}
	}
	if program == "blastn" {
		args = append(args, "-dust", yesNo(req.FilterQuery))
	} else {
		args = append(args, "-seg", yesNo(req.FilterQuery))
	}
	if matrix := strings.TrimSpace(req.ComparisonMatrix); matrix != "" && !strings.EqualFold(matrix, "default") && program != "blastn" {
		args = append(args, "-matrix", matrix)
	}
	cmd := exec.CommandContext(ctx, program, args...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return model.BlastResult{}, fmt.Errorf("%s failed: %w%s", program, err, capturedOutput(stderr.String()))
	}
	rows, err := parseBlastTabular(outPath, fastaIndex, program, queryLengths, fallbackQueryLength)
	if err != nil {
		return model.BlastResult{}, err
	}
	for i := range rows {
		rows[i].BlastProgram = strings.ToUpper(program)
	}
	return model.BlastResult{Rows: rows}, nil
}

func localBlastQueryFASTA(input string) (string, map[string]int, int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil, 0, fmt.Errorf("empty BLAST query sequence")
	}
	if !strings.HasPrefix(input, ">") {
		seq := strings.ToUpper(strings.Join(strings.Fields(input), ""))
		if seq == "" {
			return "", nil, 0, fmt.Errorf("empty BLAST query sequence")
		}
		return ">query\n" + wrapFASTA(seq), map[string]int{"query": len(seq)}, len(seq), nil
	}
	scanner := bufio.NewScanner(strings.NewReader(input))
	var out strings.Builder
	lengths := map[string]int{}
	current := ""
	seen := map[string]int{}
	var seq strings.Builder
	flush := func() error {
		if current == "" {
			return nil
		}
		s := strings.ToUpper(strings.Join(strings.Fields(seq.String()), ""))
		if s == "" {
			return fmt.Errorf("empty FASTA entry %s", current)
		}
		lengths[current] = len(s)
		out.WriteByte('>')
		out.WriteString(current)
		out.WriteByte('\n')
		out.WriteString(wrapFASTA(s))
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			if err := flush(); err != nil {
				return "", nil, 0, err
			}
			current = uniqueQueryID(strings.TrimPrefix(line, ">"), len(lengths)+1, seen)
			seq.Reset()
			continue
		}
		if current == "" {
			return "", nil, 0, fmt.Errorf("FASTA sequence appears before header")
		}
		seq.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		return "", nil, 0, err
	}
	if err := flush(); err != nil {
		return "", nil, 0, err
	}
	fallback := 0
	for _, n := range lengths {
		fallback = n
		break
	}
	return out.String(), lengths, fallback, nil
}

func uniqueQueryID(header string, index int, seen map[string]int) string {
	id := safeQueryID(header)
	if id == "" {
		id = fmt.Sprintf("query_%d", index)
	}
	key := strings.ToLower(id)
	seen[key]++
	if seen[key] > 1 {
		id = fmt.Sprintf("%s_%d", id, seen[key])
	}
	return id
}

func safeQueryID(value string) string {
	if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}
	var b strings.Builder
	last := false
	for _, r := range value {
		valid := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !valid {
			if !last {
				b.WriteByte('_')
				last = true
			}
			continue
		}
		b.WriteRune(r)
		last = r == '_'
	}
	return strings.Trim(b.String(), "._-")
}

func wrapFASTA(seq string) string {
	var b strings.Builder
	for len(seq) > 80 {
		b.WriteString(seq[:80])
		b.WriteByte('\n')
		seq = seq[80:]
	}
	if seq != "" {
		b.WriteString(seq)
		b.WriteByte('\n')
	}
	return b.String()
}

type fastaEntry struct {
	Defline string
	Length  int
}

func buildFastaIndex(path string) (map[string]fastaEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 32*1024*1024)
	index := make(map[string]fastaEntry)
	header := ""
	length := 0
	flush := func() {
		if header == "" {
			return
		}
		entry := fastaEntry{Defline: header, Length: length}
		for _, key := range identifierKeys(fastaHeaderID(header)) {
			index[key] = entry
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">") {
			flush()
			header = strings.TrimPrefix(line, ">")
			length = 0
			continue
		}
		length += len(strings.TrimSpace(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return index, nil
}

func parseBlastTabular(path string, fastaIndex map[string]fastaEntry, program string, queryLengths map[string]int, fallbackQueryLength int) ([]model.BlastResultRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	rows := make([]model.BlastResultRow, 0, 32)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) < 12 {
			continue
		}
		pident, _ := strconv.ParseFloat(fields[2], 64)
		alignLen, _ := strconv.Atoi(fields[3])
		mismatch, _ := strconv.Atoi(fields[4])
		gapOpen, _ := strconv.Atoi(fields[5])
		qstart, _ := strconv.Atoi(fields[6])
		qend, _ := strconv.Atoi(fields[7])
		sstart, _ := strconv.Atoi(fields[8])
		send, _ := strconv.Atoi(fields[9])
		bitscore, _ := strconv.ParseFloat(fields[11], 64)
		queryLength := fallbackQueryLength
		if n := queryLengths[fields[0]]; n > 0 {
			queryLength = n
		}
		row := model.BlastResultRow{
			SourceDatabase:  "tair",
			HitNumber:       len(rows) + 1,
			Protein:         fields[1],
			SubjectID:       fields[1],
			EValue:          fields[10],
			PercentIdentity: pident,
			AlignLength:     alignLen,
			QueryID:         fields[0],
			QueryFrom:       qstart,
			QueryTo:         qend,
			TargetFrom:      sstart,
			TargetTo:        send,
			Bitscore:        bitscore,
			Mismatches:      mismatch,
			GapOpenings:     gapOpen,
			Identical:       int(pident * float64(alignLen) / 100),
			Gaps:            gapOpen,
			QueryLength:     queryLength,
			SequenceID:      fields[1],
		}
		next := 12
		if program == "blastp" || program == "blastx" || program == "tblastn" {
			if len(fields) > next {
				row.Positives, _ = strconv.Atoi(fields[next])
				next++
			}
		}
		if program == "blastx" || program == "tblastn" {
			qframe, sframe := 0, 0
			if len(fields) > next {
				qframe, _ = strconv.Atoi(fields[next])
				next++
			}
			if len(fields) > next {
				sframe, _ = strconv.Atoi(fields[next])
			}
			row.Strands = frameDirection(qframe) + "/" + frameDirection(sframe)
		}
		if row.AlignLength > 0 && row.QueryLength > 0 {
			row.AlignQueryLengthPercent = float64(row.AlignLength) / float64(row.QueryLength) * 100
		}
		if entry, ok := lookupFastaEntry(fastaIndex, fields[1]); ok {
			row.Defline = entry.Defline
			row.TargetLength = entry.Length
		}
		rows = append(rows, row)
	}
	return rows, scanner.Err()
}

func enrichLocalBlastRows(ctx context.Context, c *Client, rel releaseInfo, version model.SpeciesCandidate, rows *[]model.BlastResultRow) {
	if rows == nil || len(*rows) == 0 {
		return
	}
	lookup := make(map[string]model.KeywordResultRow)
	for i := range *rows {
		row := &(*rows)[i]
		row.Species = version.DisplayLabel()
		row.JBrowseName = rel.Name
		row.TargetID = version.ProteomeID
		if hit, ok := lookup[strings.ToUpper(strings.TrimSpace(row.SequenceID))]; ok {
			row.Protein = firstNonEmpty(hit.GeneIdentifier, hit.TranscriptID, row.Protein)
			row.SequenceID = firstNonEmpty(hit.SequenceID, row.SequenceID)
			row.TranscriptID = hit.TranscriptID
			row.GeneReportURL = hit.GeneReportURL
			row.Defline = firstNonEmpty(row.Defline, hit.Description)
			row.UniProtAccession = hit.UniProt
		} else if liveHit, hitErr := c.findRow(ctx, version, row.SequenceID); hitErr == nil {
			lookup[strings.ToUpper(strings.TrimSpace(row.SequenceID))] = liveHit
			hit := liveHit
			row.Protein = firstNonEmpty(hit.GeneIdentifier, hit.TranscriptID, row.Protein)
			row.SequenceID = firstNonEmpty(hit.SequenceID, row.SequenceID)
			row.TranscriptID = hit.TranscriptID
			row.GeneReportURL = hit.GeneReportURL
			row.Defline = firstNonEmpty(row.Defline, hit.Description)
			row.UniProtAccession = hit.UniProt
		}
		if row.GeneReportURL == "" {
			row.GeneReportURL = rel.ReportURLBase + urlQueryEscape(stripTranscriptSuffix(row.SequenceID))
		}
	}
}

func lookupFastaEntry(index map[string]fastaEntry, id string) (fastaEntry, bool) {
	for _, key := range identifierKeys(id) {
		if entry, ok := index[key]; ok {
			return entry, true
		}
	}
	return fastaEntry{}, false
}

func localBlastThreads(ctx context.Context) int {
	if ctx != nil {
		if n, ok := ctx.Value(localBlastThreadsContextKey{}).(int); ok && n > 0 {
			return n
		}
	}
	if n, ok := envLocalBlastThreads(); ok {
		return n
	}
	return maxInt(1, minInt(16, runtime.NumCPU()))
}

func normalizedWordSize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "default") {
		return ""
	}
	if _, err := strconv.Atoi(value); err != nil {
		return ""
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func frameDirection(frame int) string {
	if frame < 0 {
		return "-"
	}
	if frame > 0 {
		return "+"
	}
	return "0"
}

func capturedOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 8 {
		lines = append(lines[:8], "...")
	}
	return "\n" + strings.Join(lines, "\n")
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func envLocalBlastThreads() (int, bool) {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("PHGO_TAIR_BLAST_THREADS"))); err == nil && n > 0 {
		return n, true
	}
	return 0, false
}
