package phylo

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed viewer_assets/**
var viewerAssets embed.FS

type ViewerState struct {
	mu        sync.RWMutex
	payloads  map[string]ViewerPayload
	previews  map[string]map[string]any
	states    map[string]json.RawMessage
	statuses  map[string]ViewerSessionStatus
	msaRows   map[string]map[string]string
	msaStates map[string]MSAState
	seq       uint64
	sessions  map[string]uint64
	notify    chan struct{}
}

type ViewerServer struct {
	addr       string
	state      ViewerState
	srv        *http.Server
	ln         net.Listener
	msaApplyFn func(context.Context, string, MSAApplyRequest) (MSAApplyResponse, error)
}

type ViewerSessionStatus struct {
	Refreshing bool      `json:"refreshing"`
	Message    string    `json:"message,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type MSAApplyRow struct {
	TaxonID     string `json:"taxon_id"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Sequence    string `json:"sequence,omitempty"`
	Index       *int   `json:"index,omitempty"`
	State       string `json:"state"`
}

type MSAApplyRequest struct {
	Rows []MSAApplyRow `json:"rows"`
}

type MSAApplyResponse struct {
	Accepted   bool   `json:"accepted"`
	Refreshing bool   `json:"refreshing"`
	Message    string `json:"message,omitempty"`
}

func NewViewerServer(addr string) *ViewerServer {
	return &ViewerServer{
		addr: addr,
		state: ViewerState{
			payloads:  map[string]ViewerPayload{},
			previews:  map[string]map[string]any{},
			states:    map[string]json.RawMessage{},
			statuses:  map[string]ViewerSessionStatus{},
			msaRows:   map[string]map[string]string{},
			msaStates: map[string]MSAState{},
			sessions:  map[string]uint64{},
			notify:    make(chan struct{}),
		},
	}
}

func (v *ViewerServer) SetMSAApplyHandler(fn func(context.Context, string, MSAApplyRequest) (MSAApplyResponse, error)) {
	if v == nil {
		return
	}
	v.state.mu.Lock()
	v.msaApplyFn = fn
	v.state.mu.Unlock()
}

func (v *ViewerServer) SetSessionStatus(sessionID string, status ViewerSessionStatus) {
	if v == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now()
	}
	v.state.mu.Lock()
	if v.state.statuses == nil {
		v.state.statuses = map[string]ViewerSessionStatus{}
	}
	v.state.statuses[sessionID] = status
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
}

func (v *ViewerServer) SetMSAPayload(sessionID string, payload ViewerPayload) {
	if v == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	payload.SessionID = sessionID
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = time.Now()
	}
	v.state.mu.Lock()
	if v.state.payloads == nil {
		v.state.payloads = map[string]ViewerPayload{}
	}
	sharedPayload, ok := v.state.payloads[sessionID]
	if !ok {
		v.state.payloads[sessionID] = payload
		sharedPayload = payload
	}
	v.normalizeMSAStateLocked(sessionID, sharedPayload)
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
}

func (v *ViewerServer) GetMSAState(sessionID string) MSAState {
	if v == nil {
		return MSAState{}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return MSAState{}
	}
	v.state.mu.RLock()
	state := cloneMSAState(v.state.msaStates[sessionID])
	v.state.mu.RUnlock()
	return state
}

func (v *ViewerServer) SetMSAState(sessionID string, state MSAState) {
	if v == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
	}
	v.state.mu.Lock()
	if v.state.msaStates == nil {
		v.state.msaStates = map[string]MSAState{}
	}
	if v.state.msaRows == nil {
		v.state.msaRows = map[string]map[string]string{}
	}
	existing := cloneMSAState(v.state.msaStates[sessionID])
	state = mergeMSAState(existing, state)
	v.state.msaStates[sessionID] = state
	rows := make(map[string]string, len(state.Rows))
	for _, row := range state.Rows {
		taxonID := strings.TrimSpace(row.TaxonID)
		stateValue := normalizeMSASelectionState(row.State)
		if taxonID == "" || stateValue == "" {
			continue
		}
		rows[taxonID] = stateValue
	}
	v.state.msaRows[sessionID] = rows
	payload, _ := v.msaPayloadLocked(sessionID)
	v.normalizeMSAStateLocked(sessionID, payload)
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
}

func (v *ViewerServer) Start(ctx context.Context) error {
	if strings.TrimSpace(v.addr) == "" {
		v.addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", v.addr)
	if err != nil {
		return err
	}
	v.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/health", v.handleHealth)
	mux.HandleFunc("/sessions/", v.handleSession)
	mux.HandleFunc("/events/", v.handleEvents)
	mux.HandleFunc("/msa/", v.handleMSAPage)
	mux.HandleFunc("/assets/", v.handleAsset)
	mux.HandleFunc("/jalview-bootstrap/", v.handleJalviewBootstrapPage)
	mux.HandleFunc("/jalview-bootstrap.html", v.handleAsset)
	mux.HandleFunc("/phgo-icon.png", v.handleAsset)
	mux.HandleFunc("/favicon.ico", v.handleAsset)
	v.srv = &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = v.srv.Shutdown(context.Background())
	}()
	go func() {
		_ = v.srv.Serve(ln)
	}()
	return nil
}

func (v *ViewerServer) URL() string {
	if v == nil || v.ln == nil {
		return ""
	}
	return "http://" + v.ln.Addr().String()
}

func (v *ViewerServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (v *ViewerServer) handleSession(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/sessions/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	sessionID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		v.handleSessionPage(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "tree" && r.Method == http.MethodGet:
		v.handleSessionPage(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "msa" && r.Method == http.MethodGet:
		v.handleMSASessionPage(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "aligned.fasta" && r.Method == http.MethodGet:
		v.handleAlignedFASTAGet(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "payload" && r.Method == http.MethodGet:
		v.handlePayloadGet(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "payload" && r.Method == http.MethodPut:
		v.handlePayloadPut(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "preview" && r.Method == http.MethodGet:
		v.handlePreviewGet(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "preview" && r.Method == http.MethodPut:
		v.handlePreviewPut(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "state" && r.Method == http.MethodGet:
		v.handleViewerStateGet(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "state" && r.Method == http.MethodPut:
		v.handleViewerStatePut(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodGet:
		v.handleStatusGet(w, r, sessionID)
	case len(parts) == 3 && parts[1] == "msa" && parts[2] == "state" && r.Method == http.MethodGet:
		v.handleMSAStateGet(w, r, sessionID)
	case len(parts) == 3 && parts[1] == "msa" && parts[2] == "state" && r.Method == http.MethodPut:
		v.handleMSAStatePut(w, r, sessionID)
	case len(parts) == 3 && parts[1] == "msa" && parts[2] == "selection" && r.Method == http.MethodGet:
		v.handleMSASelectionGet(w, r, sessionID)
	case len(parts) == 3 && parts[1] == "msa" && parts[2] == "apply" && r.Method == http.MethodPost:
		v.handleMSAApplyPost(w, r, sessionID)
	default:
		http.NotFound(w, r)
	}
}

func (v *ViewerServer) handleSessionPage(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	payload := v.state.payloads[sessionID]
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	empty := payload.SessionID == "" || !strings.EqualFold(payload.SessionID, sessionID)
	state := "empty"
	if !empty {
		state = "ready"
	}
	html, err := viewerAsset("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	html = bytesReplaceAll(html, []byte("__PHGO_SESSION__"), []byte(htmlEscape(sessionID)))
	html = bytesReplaceAll(html, []byte("__PHGO_STATE__"), []byte(htmlEscape(state)))
	html = bytesReplaceAll(html, []byte("__PHGO_SEQ__"), []byte(strconv.FormatUint(seq, 10)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setViewerNoStoreHeaders(w)
	_, _ = w.Write(html)
}

func (v *ViewerServer) handlePayloadGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	payload, ok := v.state.payloads[sessionID]
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	if !ok {
		payload = ViewerPayload{SchemaVersion: 1, SessionID: sessionID, UpdatedAt: time.Now()}
	}
	payload = normalizeJalviewPayloadMetadata(payload)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", strconv.FormatUint(seq, 10))
	_ = json.NewEncoder(w).Encode(payload)
}

func normalizeJalviewPayloadMetadata(payload ViewerPayload) ViewerPayload {
	switch payload.Metadata.ConversionTarget {
	case ConversionTargetProtein:
		payload.Metadata.SequenceKind = SequenceProtein
	case ConversionTargetDNA:
		payload.Metadata.SequenceKind = SequenceNucleotide
	}
	if payload.Metadata.SequenceKind == "" || payload.Metadata.SequenceKind == SequenceUnknown {
		payload.Metadata.SequenceKind = dominantRecordSequenceKind(payload.Metadata.Records)
	}
	if payload.Metadata.ConversionTarget == "" {
		switch payload.Metadata.SequenceKind {
		case SequenceProtein:
			payload.Metadata.ConversionTarget = ConversionTargetProtein
		case SequenceNucleotide:
			payload.Metadata.ConversionTarget = ConversionTargetDNA
		}
	}
	return payload
}

func dominantRecordSequenceKind(records []InputRecord) SequenceKind {
	protein := 0
	nucleotide := 0
	for _, record := range records {
		switch record.SequenceKind {
		case SequenceProtein:
			protein++
		case SequenceNucleotide:
			nucleotide++
		}
	}
	switch {
	case protein > 0 && protein >= nucleotide:
		return SequenceProtein
	case nucleotide > 0:
		return SequenceNucleotide
	default:
		return SequenceUnknown
	}
}

func (v *ViewerServer) handleAlignedFASTAGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	payload, ok := v.msaPayloadLocked(sessionID)
	state := cloneMSAState(v.state.msaStates[sessionID])
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	if !ok || strings.TrimSpace(payload.AlignedFASTA) == "" {
		http.Error(w, "aligned FASTA is unavailable for this session", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("ETag", strconv.FormatUint(seq, 10))
	setViewerNoStoreHeaders(w)
	_, _ = w.Write([]byte(jalviewAlignedFASTAWithState(payload, state)))
}

func jalviewAlignedFASTA(payload ViewerPayload) string {
	return jalviewAlignedFASTAWithState(payload, MSAState{})
}

func jalviewAlignedFASTAWithState(payload ViewerPayload, state MSAState) string {
	if len(payload.Metadata.Records) == 0 || strings.TrimSpace(payload.AlignedFASTA) == "" {
		return payload.AlignedFASTA
	}
	names := make(map[string]InputRecord, len(payload.Metadata.Records))
	for _, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" || strings.TrimSpace(jalviewRecordDisplayName(record)) == "" {
			continue
		}
		names[taxonID] = record
	}
	if len(names) == 0 {
		return payload.AlignedFASTA
	}
	stateByTaxon, stateByName := msaSequenceStateIndexes(state.Sequences)
	var b strings.Builder
	var overrideSequence string
	overrideWritten := false
	for _, line := range strings.SplitAfter(payload.AlignedFASTA, "\n") {
		content := strings.TrimRight(line, "\r\n")
		eol := line[len(content):]
		if strings.HasPrefix(content, ">") {
			overrideSequence = ""
			overrideWritten = false
			header := strings.TrimSpace(strings.TrimPrefix(content, ">"))
			if record, ok := names[fastaHeaderID(header)]; ok {
				sequenceState, _ := msaSequenceStateForRecord(record, stateByTaxon, stateByName)
				content = ">" + jalviewPHgoHeaderWithState(record, sequenceState)
				overrideSequence = fastaSequenceText(sequenceState.Sequence)
			}
		} else if overrideSequence != "" {
			if !overrideWritten {
				writeWrappedFASTASequence(&b, overrideSequence, eol)
				overrideWritten = true
			}
			continue
		}
		b.WriteString(content)
		b.WriteString(eol)
	}
	return b.String()
}

func fastaHeaderID(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return header
	}
	return fields[0]
}

func jalviewPHgoHeader(record InputRecord) string {
	return jalviewPHgoHeaderWithState(record, MSASequenceState{})
}

func jalviewPHgoHeaderWithState(record InputRecord, state MSASequenceState) string {
	displayName := strings.TrimSpace(jalviewRecordDisplayName(record))
	if savedName := strings.TrimSpace(state.DisplayName); savedName != "" {
		displayName = savedName
	}
	name := base64.RawURLEncoding.EncodeToString([]byte(displayName))
	taxonIDValue := strings.TrimSpace(record.TaxonID)
	if taxonIDValue == "" {
		taxonIDValue = strings.TrimSpace(state.TaxonID)
	}
	taxonID := base64.RawURLEncoding.EncodeToString([]byte(taxonIDValue))
	if taxonID == "" {
		return "phgo_name64:" + name
	}
	return "phgo_name64:" + name + " phgo_taxon_id64:" + taxonID
}

func msaSequenceStateIndexes(sequences []MSASequenceState) (map[string]MSASequenceState, map[string]MSASequenceState) {
	byTaxon := make(map[string]MSASequenceState, len(sequences))
	byName := make(map[string]MSASequenceState, len(sequences))
	for _, seq := range sequences {
		seq.TaxonID = strings.TrimSpace(seq.TaxonID)
		seq.DisplayName = strings.TrimSpace(seq.DisplayName)
		seq.Description = strings.TrimSpace(seq.Description)
		seq.Sequence = strings.TrimSpace(seq.Sequence)
		if seq.TaxonID != "" {
			byTaxon[seq.TaxonID] = seq
		}
		if seq.DisplayName != "" {
			byName[seq.DisplayName] = seq
		}
	}
	return byTaxon, byName
}

func msaSequenceStateForRecord(record InputRecord, byTaxon map[string]MSASequenceState, byName map[string]MSASequenceState) (MSASequenceState, bool) {
	if seq, ok := byTaxon[strings.TrimSpace(record.TaxonID)]; ok {
		return seq, true
	}
	if seq, ok := byName[jalviewRecordDisplayName(record)]; ok {
		return seq, true
	}
	return MSASequenceState{}, false
}

func fastaSequenceText(sequence string) string {
	return strings.Join(strings.Fields(sequence), "")
}

func writeWrappedFASTASequence(b *strings.Builder, sequence string, eol string) {
	if sequence == "" {
		return
	}
	if eol == "" {
		eol = "\n"
	}
	const width = 80
	for len(sequence) > width {
		b.WriteString(sequence[:width])
		b.WriteString(eol)
		sequence = sequence[width:]
	}
	b.WriteString(sequence)
	b.WriteString(eol)
}

func jalviewRecordDisplayName(record InputRecord) string {
	if name := strings.TrimSpace(record.BaseDisplayName); name != "" {
		return name
	}
	return strings.TrimSpace(record.DisplayName)
}

func inputRecordDisplayLabel(record InputRecord) string {
	name := strings.TrimSpace(jalviewRecordDisplayName(record))
	prefix := strings.TrimSpace(record.DisplayPrefix)
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + " " + name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func (v *ViewerServer) handlePayloadPut(w http.ResponseWriter, r *http.Request, sessionID string) {
	defer r.Body.Close()
	var payload ViewerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.SessionID = strings.TrimSpace(sessionID)
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = time.Now()
	}
	v.state.mu.Lock()
	v.state.payloads[sessionID] = payload
	v.normalizeMSAStateLocked(sessionID, payload)
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (v *ViewerServer) handlePreviewGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	preview := clonePreview(v.state.previews[sessionID])
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	if preview == nil {
		preview = map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", strconv.FormatUint(seq, 10))
	_ = json.NewEncoder(w).Encode(preview)
}

func (v *ViewerServer) handlePreviewPut(w http.ResponseWriter, r *http.Request, sessionID string) {
	defer r.Body.Close()
	var preview map[string]any
	if err := json.NewDecoder(r.Body).Decode(&preview); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	v.state.mu.Lock()
	if v.state.previews == nil {
		v.state.previews = map[string]map[string]any{}
	}
	if v.state.previews[sessionID] == nil {
		v.state.previews[sessionID] = map[string]any{}
	}
	for key, value := range preview {
		v.state.previews[sessionID][key] = value
	}
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (v *ViewerServer) handleViewerStateGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	state := cloneRawJSON(v.state.states[sessionID])
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	if len(bytes.TrimSpace(state)) == 0 {
		state = json.RawMessage(`{}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", strconv.FormatUint(seq, 10))
	_, _ = w.Write(state)
}

func (v *ViewerServer) handleViewerStatePut(w http.ResponseWriter, r *http.Request, sessionID string) {
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if !json.Valid(raw) {
		http.Error(w, "viewer state must be valid JSON", http.StatusBadRequest)
		return
	}
	v.state.mu.Lock()
	if v.state.states == nil {
		v.state.states = map[string]json.RawMessage{}
	}
	v.state.states[sessionID] = cloneRawJSON(raw)
	v.state.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (v *ViewerServer) handleStatusGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	status := v.state.statuses[sessionID]
	v.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (v *ViewerServer) handleMSAStateGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	state := cloneMSAState(v.state.msaStates[sessionID])
	seq := v.state.sessions[sessionID]
	v.state.mu.RUnlock()
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", strconv.FormatUint(seq, 10))
	setViewerNoStoreHeaders(w)
	_ = json.NewEncoder(w).Encode(state)
}

func (v *ViewerServer) handleMSAStatePut(w http.ResponseWriter, r *http.Request, sessionID string) {
	defer r.Body.Close()
	var state MSAState
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20))
	if err := decoder.Decode(&state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = now
	}
	v.state.mu.Lock()
	if v.state.msaStates == nil {
		v.state.msaStates = map[string]MSAState{}
	}
	if v.state.msaRows == nil {
		v.state.msaRows = map[string]map[string]string{}
	}
	existing := cloneMSAState(v.state.msaStates[sessionID])
	merged := mergeMSAState(existing, state)
	if merged.UpdatedAt.IsZero() {
		merged.UpdatedAt = now
	}
	v.state.msaStates[sessionID] = merged
	if len(state.Rows) > 0 {
		rows := make(map[string]string, len(state.Rows))
		for _, row := range state.Rows {
			taxonID := strings.TrimSpace(row.TaxonID)
			stateValue := normalizeMSASelectionState(row.State)
			if taxonID == "" || stateValue == "" {
				continue
			}
			rows[taxonID] = stateValue
		}
		v.state.msaRows[sessionID] = rows
	}
	payload, _ := v.msaPayloadLocked(sessionID)
	v.normalizeMSAStateLocked(sessionID, payload)
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (v *ViewerServer) handleMSASelectionGet(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	payload, _ := v.msaPayloadLocked(sessionID)
	rowStates := cloneViewerStringMap(v.msaRowsLocked(sessionID))
	v.state.mu.RUnlock()
	type row struct {
		TaxonID         string `json:"taxon_id"`
		DisplayName     string `json:"display_name"`
		DisplayPrefix   string `json:"display_prefix,omitempty"`
		DisplayLabel    string `json:"display_label,omitempty"`
		CanvasItemIndex int    `json:"canvas_item_index,omitempty"`
		CanvasRow       int    `json:"canvas_row,omitempty"`
		Index           int    `json:"index"`
		State           string `json:"state"`
	}
	rows := make([]row, 0, len(payload.Metadata.Records))
	for index, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" {
			continue
		}
		state := normalizeMSASelectionState(rowStates[taxonID])
		if state == "" {
			state = "green"
		}
		rows = append(rows, row{
			TaxonID:       taxonID,
			DisplayName:   jalviewRecordDisplayName(record),
			DisplayPrefix: strings.TrimSpace(record.DisplayPrefix),
			DisplayLabel:  inputRecordDisplayLabel(record),
			// The browser/DSL contract is one-based ([item,row]).  Keep the
			// zero-based values inside InputRecord for Canvas operations, but do
			// not leak them through the public MSA selection API.
			CanvasItemIndex: record.CanvasItemIndex + 1,
			CanvasRow:       record.CanvasRow + 1,
			Index:           index,
			State:           state,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
}

func (v *ViewerServer) handleMSAApplyPost(w http.ResponseWriter, r *http.Request, sessionID string) {
	defer r.Body.Close()
	var req MSAApplyRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Rows) > 10000 {
		http.Error(w, "MSA apply contains too many rows", http.StatusRequestEntityTooLarge)
		return
	}
	v.state.mu.RLock()
	payload, _ := v.msaPayloadLocked(sessionID)
	v.state.mu.RUnlock()
	taxonByDisplayName := make(map[string]string, len(payload.Metadata.Records))
	taxonByIndex := make(map[int]string, len(payload.Metadata.Records))
	for index, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" {
			continue
		}
		taxonByIndex[index] = taxonID
		if displayName := strings.TrimSpace(jalviewRecordDisplayName(record)); displayName != "" {
			taxonByDisplayName[displayName] = taxonID
		}
	}
	cleaned := make(map[string]string, len(req.Rows))
	cleanedEdits := make(map[string]MSAApplyRow, len(req.Rows))
	cleanedOrder := make([]string, 0, len(req.Rows))
	for _, row := range req.Rows {
		taxonID := strings.TrimSpace(row.TaxonID)
		state := normalizeMSASelectionState(row.State)
		name := strings.TrimSpace(firstNonEmpty(row.DisplayName, row.Name))
		if taxonID == "" && name != "" {
			taxonID = taxonByDisplayName[name]
		}
		if taxonID == "" && row.Index != nil {
			taxonID = taxonByIndex[*row.Index]
		}
		if taxonID == "" || state == "" {
			continue
		}
		if _, exists := cleaned[taxonID]; !exists {
			cleanedOrder = append(cleanedOrder, taxonID)
		}
		row.TaxonID = taxonID
		row.State = state
		row.Name = name
		row.DisplayName = name
		row.Description = strings.TrimSpace(row.Description)
		row.Sequence = strings.TrimSpace(row.Sequence)
		cleaned[taxonID] = state
		cleanedEdits[taxonID] = row
	}
	v.state.mu.Lock()
	if v.state.msaRows == nil {
		v.state.msaRows = map[string]map[string]string{}
	}
	v.state.msaRows[sessionID] = cleaned
	v.normalizeMSAStateLocked(sessionID, payload)
	applyFn := v.msaApplyFn
	v.state.seq++
	v.state.sessions[sessionID] = v.state.seq
	v.state.broadcastLocked()
	v.state.mu.Unlock()
	if applyFn == nil {
		http.Error(w, "MSA apply is unavailable for this viewer session", http.StatusServiceUnavailable)
		return
	}
	cleanReq := MSAApplyRequest{Rows: make([]MSAApplyRow, 0, len(cleaned))}
	if len(payload.Metadata.Records) > 0 {
		for index, record := range payload.Metadata.Records {
			taxonID := strings.TrimSpace(record.TaxonID)
			state, ok := cleaned[taxonID]
			if !ok {
				continue
			}
			rowIndex := index
			edit := cleanedEdits[taxonID]
			name := strings.TrimSpace(firstNonEmpty(edit.DisplayName, edit.Name, jalviewRecordDisplayName(record)))
			cleanReq.Rows = append(cleanReq.Rows, MSAApplyRow{
				TaxonID:     taxonID,
				Name:        name,
				DisplayName: name,
				Description: edit.Description,
				Sequence:    edit.Sequence,
				Index:       &rowIndex,
				State:       state,
			})
		}
	} else {
		for _, taxonID := range cleanedOrder {
			edit := cleanedEdits[taxonID]
			name := strings.TrimSpace(firstNonEmpty(edit.DisplayName, edit.Name))
			cleanReq.Rows = append(cleanReq.Rows, MSAApplyRow{
				TaxonID:     taxonID,
				Name:        name,
				DisplayName: name,
				Description: edit.Description,
				Sequence:    edit.Sequence,
				State:       cleaned[taxonID],
			})
		}
	}
	resp, err := applyFn(r.Context(), sessionID, cleanReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (v *ViewerServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/events/"), "/")
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	writeEvent := func() {
		v.state.mu.RLock()
		seq := v.state.sessions[sessionID]
		v.state.mu.RUnlock()
		_, _ = fmt.Fprintf(w, "event: update\ndata: {\"session_id\":%q,\"seq\":%d}\n\n", sessionID, seq)
		flusher.Flush()
	}
	writeEvent()
	for {
		v.state.mu.RLock()
		notify := v.state.notify
		v.state.mu.RUnlock()
		select {
		case <-r.Context().Done():
			return
		case <-notify:
			writeEvent()
		}
	}
}

func (v *ViewerServer) handleMSAPage(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/msa/"), "/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, r)
		return
	}
	v.handleMSASessionPage(w, r, sessionID)
}

func (v *ViewerServer) handleMSASessionPage(w http.ResponseWriter, r *http.Request, sessionID string) {
	v.state.mu.RLock()
	payload, _ := v.msaPayloadLocked(sessionID)
	v.state.mu.RUnlock()
	payload = normalizeJalviewPayloadMetadata(payload)
	html, err := v.jalviewBootstrapHTML(msaJalviewInitialState(sessionID, payload))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setViewerNoStoreHeaders(w)
	_, _ = w.Write(html)
}

func (v *ViewerServer) msaPayloadLocked(sessionID string) (ViewerPayload, bool) {
	if v == nil {
		return ViewerPayload{}, false
	}
	payload, ok := v.state.payloads[sessionID]
	return payload, ok
}

func (v *ViewerServer) msaRowsLocked(sessionID string) map[string]string {
	if v == nil {
		return nil
	}
	if rows := v.state.msaRows[sessionID]; len(rows) > 0 {
		return rows
	}
	state := v.state.msaStates[sessionID]
	if len(state.Rows) == 0 {
		return nil
	}
	rows := make(map[string]string, len(state.Rows))
	for _, row := range state.Rows {
		taxonID := strings.TrimSpace(row.TaxonID)
		stateValue := normalizeMSASelectionState(row.State)
		if taxonID != "" && stateValue != "" {
			rows[taxonID] = stateValue
		}
	}
	return rows
}

func (v *ViewerServer) normalizeMSAStateLocked(sessionID string, payload ViewerPayload) {
	if v == nil {
		return
	}
	if v.state.msaRows == nil {
		v.state.msaRows = map[string]map[string]string{}
	}
	if v.state.msaStates == nil {
		v.state.msaStates = map[string]MSAState{}
	}
	previous := v.msaRowsLocked(sessionID)
	rows := make(map[string]string, len(payload.Metadata.Records))
	stateRows := make([]MSASelectionRow, 0, len(payload.Metadata.Records))
	if len(payload.Metadata.Records) == 0 {
		keys := make([]string, 0, len(previous))
		for taxonID := range previous {
			keys = append(keys, taxonID)
		}
		sort.Strings(keys)
		for index, taxonID := range keys {
			state := normalizeMSASelectionState(previous[taxonID])
			if state == "" {
				continue
			}
			rows[taxonID] = state
			stateRows = append(stateRows, MSASelectionRow{TaxonID: taxonID, Index: index, State: state})
		}
		existing := cloneMSAState(v.state.msaStates[sessionID])
		existing.SchemaVersion = 2
		existing.UpdatedAt = time.Now()
		existing.Rows = stateRows
		existing.Sequences = normalizeMSASequencesForPayload(existing.Sequences, payload)
		v.state.msaRows[sessionID] = rows
		v.state.msaStates[sessionID] = existing
		return
	}
	for index, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" {
			continue
		}
		state := normalizeMSASelectionState(previous[taxonID])
		if state == "" {
			state = "green"
		}
		rows[taxonID] = state
		stateRows = append(stateRows, MSASelectionRow{
			TaxonID:         taxonID,
			DisplayName:     jalviewRecordDisplayName(record),
			DisplayPrefix:   strings.TrimSpace(record.DisplayPrefix),
			DisplayLabel:    inputRecordDisplayLabel(record),
			CanvasItemIndex: record.CanvasItemIndex,
			CanvasRow:       record.CanvasRow,
			Index:           index,
			State:           state,
		})
	}
	existing := cloneMSAState(v.state.msaStates[sessionID])
	existing.SchemaVersion = 2
	existing.UpdatedAt = time.Now()
	existing.Rows = stateRows
	existing.Sequences = normalizeMSASequencesForPayload(existing.Sequences, payload)
	v.state.msaRows[sessionID] = rows
	v.state.msaStates[sessionID] = existing
}

func (v *ViewerServer) handleJalviewBootstrapPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		http.Error(w, "JalviewJS bootstrap must not be served with query parameters", http.StatusBadRequest)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/jalview-bootstrap/"), "/")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") || path.Ext(name) != ".html" {
		http.NotFound(w, r)
		return
	}
	html, err := v.jalviewBootstrapHTML(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setViewerNoStoreHeaders(w)
	_, _ = w.Write(html)
}

func (v *ViewerServer) jalviewBootstrapHTML(initialState map[string]string) ([]byte, error) {
	html, err := viewerAsset("jalview-bootstrap.html")
	if err != nil {
		return nil, err
	}
	if initialState == nil {
		return html, nil
	}
	stateJSON, err := json.Marshal(initialState)
	if err != nil {
		return nil, err
	}
	return bytesReplaceAll(html, []byte("window.__PHGOJalviewInitialState = null;"), []byte("window.__PHGOJalviewInitialState = "+string(stateJSON)+";")), nil
}

func jalviewBootstrapURL(initialState map[string]string) (string, error) {
	stateJSON, err := json.Marshal(initialState)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(stateJSON)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	return "/jalview-bootstrap/" + url.PathEscape(stamp) + ".html#phgo=" + encoded, nil
}

func msaJalviewInitialState(sessionID string, payload ViewerPayload) map[string]string {
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = strings.TrimSpace(payload.Metadata.TreeComputationSource)
	}
	if title == "" {
		title = sessionID
	}
	return map[string]string{
		"session":          sessionID,
		"open":             "/sessions/" + sessionID + "/aligned.fasta",
		"title":            "Phgomsar: " + title,
		"sequenceKind":     string(jalviewAlignmentSequenceKind(payload.Metadata)),
		"conversionTarget": string(payload.Metadata.ConversionTarget),
		"alignmentMethod":  string(payload.Metadata.AlignmentMethod),
		"treeMethod":       string(payload.Metadata.TreeMethod),
		"payloadUpdatedAt": payload.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func jalviewAlignmentSequenceKind(metadata Metadata) SequenceKind {
	switch metadata.ConversionTarget {
	case ConversionTargetProtein:
		return SequenceProtein
	case ConversionTargetDNA:
		return SequenceNucleotide
	}
	switch metadata.SequenceKind {
	case SequenceProtein:
		return SequenceProtein
	case SequenceNucleotide:
		return SequenceNucleotide
	default:
		return SequenceUnknown
	}
}

func (v *ViewerServer) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := viewerAsset(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	default:
		w.Header().Set("Content-Type", http.DetectContentType(data))
	}
	setViewerNoStoreHeaders(w)
	_, _ = w.Write(data)
}

func viewerAsset(name string) ([]byte, error) {
	name = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(name)), "/")
	if strings.EqualFold(name, "favicon.ico") {
		name = "phgo-icon.png"
	}
	if name == "" || name == "." || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid viewer asset path")
	}
	return viewerAssets.ReadFile(path.Join("viewer_assets", name))
}

func setViewerNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (v *ViewerState) broadcastLocked() {
	old := v.notify
	v.notify = make(chan struct{})
	close(old)
}

func clonePreview(preview map[string]any) map[string]any {
	if len(preview) == 0 {
		return nil
	}
	out := make(map[string]any, len(preview))
	for key, value := range preview {
		out[key] = value
	}
	return out
}

func cloneViewerStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneMSAState(in MSAState) MSAState {
	out := in
	out.Rows = append([]MSASelectionRow(nil), in.Rows...)
	out.Sequences = append([]MSASequenceState(nil), in.Sequences...)
	out.ViewerState = cloneAnyMap(in.ViewerState)
	out.Settings = cloneAnyMap(in.Settings)
	out.Colours = cloneAnyMap(in.Colours)
	out.Annotations = cloneAnyMapSlice(in.Annotations)
	out.Features = cloneAnyMapSlice(in.Features)
	out.Groups = cloneAnyMapSlice(in.Groups)
	out.Markers = cloneAnyMapSlice(in.Markers)
	return out
}

func mergeMSAState(base MSAState, update MSAState) MSAState {
	out := cloneMSAState(update)
	if update.SchemaVersion != 0 {
		out.SchemaVersion = update.SchemaVersion
	}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = 2
	}
	if update.UpdatedAt.IsZero() {
		out.UpdatedAt = base.UpdatedAt
	}
	if update.Rows == nil {
		out.Rows = append([]MSASelectionRow(nil), base.Rows...)
	}
	if update.Sequences == nil {
		out.Sequences = append([]MSASequenceState(nil), base.Sequences...)
	}
	if update.ViewerState == nil {
		out.ViewerState = cloneAnyMap(base.ViewerState)
	}
	if update.Settings == nil {
		out.Settings = cloneAnyMap(base.Settings)
	}
	if update.Colours == nil {
		out.Colours = cloneAnyMap(base.Colours)
	}
	if update.Annotations == nil {
		out.Annotations = cloneAnyMapSlice(base.Annotations)
	}
	if update.Features == nil {
		out.Features = cloneAnyMapSlice(base.Features)
	}
	if update.Groups == nil {
		out.Groups = cloneAnyMapSlice(base.Groups)
	}
	if update.Markers == nil {
		out.Markers = cloneAnyMapSlice(base.Markers)
	}
	return out
}

func normalizeMSASequencesForPayload(sequences []MSASequenceState, payload ViewerPayload) []MSASequenceState {
	if len(sequences) == 0 {
		return nil
	}
	byTaxon := make(map[string]MSASequenceState, len(sequences))
	byName := make(map[string]MSASequenceState, len(sequences))
	for _, seq := range sequences {
		taxonID := strings.TrimSpace(seq.TaxonID)
		name := strings.TrimSpace(seq.DisplayName)
		seq.TaxonID = taxonID
		seq.DisplayName = name
		seq.Description = strings.TrimSpace(seq.Description)
		seq.Sequence = strings.TrimSpace(seq.Sequence)
		if taxonID != "" {
			byTaxon[taxonID] = seq
		}
		if name != "" {
			byName[name] = seq
		}
	}
	if len(payload.Metadata.Records) == 0 {
		out := make([]MSASequenceState, 0, len(sequences))
		keys := make([]string, 0, len(byTaxon))
		for taxonID := range byTaxon {
			keys = append(keys, taxonID)
		}
		sort.Strings(keys)
		for index, taxonID := range keys {
			seq := byTaxon[taxonID]
			seq.Index = index
			out = append(out, seq)
		}
		return out
	}
	out := make([]MSASequenceState, 0, len(payload.Metadata.Records))
	for index, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" {
			continue
		}
		seq, ok := byTaxon[taxonID]
		if !ok {
			seq, ok = byName[jalviewRecordDisplayName(record)]
		}
		if !ok {
			continue
		}
		seq.TaxonID = taxonID
		if strings.TrimSpace(seq.DisplayName) == "" {
			seq.DisplayName = jalviewRecordDisplayName(record)
		}
		seq.Index = index
		out = append(out, seq)
	}
	return out
}

func cloneAnyMapSlice(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]any, len(in))
	for i := range in {
		out[i] = cloneAnyMap(in[i])
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []map[string]any:
		return cloneAnyMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneAnyValue(typed[i])
		}
		return out
	default:
		return typed
	}
}

func normalizeMSASelectionState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "green", "selected", "checked", "on", "true":
		return "green"
	case "yellow", "msa_unselected", "msa-unselected", "excluded":
		return "yellow"
	case "red", "unselected", "unchecked", "off", "false":
		return "red"
	default:
		return ""
	}
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}

func bytesReplaceAll(data []byte, old []byte, replacement []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), string(old), string(replacement)))
}
