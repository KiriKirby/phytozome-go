(function(){var P$=Clazz.newPackage("jalview.analysis"),I$=[[0,'com.stevesoft.pat.Regex','jalview.datamodel.SearchResults','java.util.Vector','jalview.datamodel.Range','jalview.analysis.AlignSeq','jalview.util.Comparison']],$I$=function(i){return I$[i]||(I$[i]=Clazz.load(I$[0][i]))};
var C$=Clazz.newClass(P$, "Finder", null, null, 'jalview.api.FinderI');

C$.$clinit$ = function() {Clazz.load(C$, 1);
}

Clazz.newMeth(C$, '$init0$', function () {
var c;if((c = C$.superclazz) && (c = c.$init0$))c.apply(this);
this.searchResults=null;
this.idMatches=null;
this.viewport=null;
this.sequenceIndex=0;
this.columnIndex=0;
this.phgoMatchCase=false;
this.phgoTailMapMode=false;
this.phgoTailAnchors=[];
this.phgoTailTokens=[];
this.phgoTailMappedColumns=[];
}, 1);

Clazz.newMeth(C$, '$init$', function () {
}, 1);

Clazz.newMeth(C$, 'c$$jalview_api_AlignViewportI', function (av) {
C$.$init$.apply(this);
this.viewport=av;
this.sequenceIndex=0;
this.columnIndex=-1;
}, 1);

Clazz.newMeth(C$, 'findAll$S$Z$Z', function (theSearchString, matchCase, searchDescription) {
this.sequenceIndex=0;
this.columnIndex=-1;
this.doFind$S$Z$Z$Z(theSearchString, matchCase, searchDescription, true);
this.sequenceIndex=0;
this.columnIndex=-1;
});

Clazz.newMeth(C$, 'findNext$S$Z$Z', function (theSearchString, matchCase, searchDescription) {
this.doFind$S$Z$Z$Z(theSearchString, matchCase, searchDescription, false);
if (this.searchResults.isEmpty$() && this.idMatches.isEmpty$() ) {
this.sequenceIndex=0;
this.columnIndex=-1;
}});

Clazz.newMeth(C$, 'doFind$S$Z$Z$Z', function (theSearchString, matchCase, searchDescription, findAll) {
var searchString=matchCase ? theSearchString : theSearchString.toUpperCase$();
this.phgoIdPattern=Clazz.new_($I$(1).c$$S,[searchString]);
this.phgoIdPattern.setIgnoreCase$Z(!matchCase);
var searchPattern=C$.parsePhgoMotif$S(searchString);
this.phgoMatchCase=matchCase;
this.phgoTailTokens=searchPattern;
this.searchResults=Clazz.new_($I$(2));
C$.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(this.searchResults);
this.idMatches=Clazz.new_($I$(3));
var selection=this.viewport.getSelectionGroup$();
if (selection != null  && selection.getSize$() < 1 ) {
selection=null;
}var alignment=this.viewport.getAlignment$();
this.phgoTailAnchors=this.phgoTailMapMode ? C$.buildPhgoTailAnchors$jalview_api_AlignViewportI$OA$Z(this.viewport, searchPattern, matchCase) : [];
this.phgoTailMappedColumns=this.phgoTailMapMode ? C$.buildPhgoTailMappedColumns$jalview_api_AlignViewportI$OA$OA$Z(this.viewport, this.phgoTailAnchors, searchPattern, matchCase) : [];
var end=alignment.getHeight$();
while (this.sequenceIndex < end){
var seq=alignment.getSequenceAt$I(this.sequenceIndex);
var found=this.findNextMatch$jalview_datamodel_SequenceI$S$com_stevesoft_pat_Regex$Z(seq, searchString, searchPattern, searchDescription);
if (found && !findAll ) {
return;
}if (!found) {
this.sequenceIndex++;
this.columnIndex=-1;
}}
});

Clazz.newMeth(C$, 'getNextVisibleSequenceRegion$jalview_datamodel_SequenceI$I', function (sequence, column) {
var seqColStart=column;
var seqColEnd=sequence.getLength$() - 1;
var alignment=this.viewport.getAlignment$();
var visibleRegions=alignment.getHiddenColumns$().getVisContigsIterator$I$I$Z(column, alignment.getWidth$(), false);
var visible=visibleRegions.hasNext$() ? visibleRegions.next$() : null;
if (visible == null ) {
this.columnIndex=seqColEnd + 1;
return null;
}seqColStart=Math.max(seqColStart, visible[0]);
seqColEnd=Math.min(seqColEnd, visible[1]);
var selection=this.viewport.getSelectionGroup$();
if (selection != null ) {
var selectionStart=selection.getStartRes$();
var selectionEnd=selection.getEndRes$();
if (selectionStart > seqColEnd || selectionEnd < seqColStart ) {
this.columnIndex=seqColEnd + 1;
return null;
}seqColStart=Math.max(seqColStart, selectionStart);
seqColEnd=Math.min(seqColEnd, selectionEnd);
}return Clazz.new_($I$(4).c$$I$I,[seqColStart, seqColEnd]);
});

Clazz.newMeth(C$, 'findNextMatch$jalview_datamodel_SequenceI$S$com_stevesoft_pat_Regex$Z', function (seq, searchString, searchPattern, matchDescription) {
var selection=this.viewport.getSelectionGroup$();
if (selection != null  && !selection.contains$jalview_datamodel_SequenceI(seq) ) {
return false;
}if (this.columnIndex < 0) {
if (this.doNonMotifSearches$jalview_datamodel_SequenceI$S$com_stevesoft_pat_Regex$Z(seq, searchString, searchPattern, matchDescription)) {
return true;
}}var end=seq.getLength$();
while (this.columnIndex < end){
if (this.searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(seq, searchPattern)) {
return true;
}}
return false;
});

Clazz.newMeth(C$, 'searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex', function (seq, searchPattern) {
var visible=this.getNextVisibleSequenceRegion$jalview_datamodel_SequenceI$I(seq, this.columnIndex);
if (visible == null ) {
return false;
}
if (this.phgoTailMapMode) {
var mappedColumns=[];
var mappedRows=this.phgoTailMappedColumns || [];
for (var mappedRowIndex=0; mappedRowIndex < mappedRows.length; mappedRowIndex++) {
if (mappedRows[mappedRowIndex].sequence === seq) {
mappedColumns=mappedRows[mappedRowIndex].columns;
break;
}}
var minimumColumn=Math.max(visible.start, this.columnIndex);
for (var mappedColumnIndex=0; mappedColumnIndex < mappedColumns.length; mappedColumnIndex++) {
var column=mappedColumns[mappedColumnIndex];
if (column < minimumColumn) continue;
if (column > visible.end) break;
return this.recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I(seq, column, column);
}
this.columnIndex=visible.end + 1;
return false;
}
var seqString=seq.getSequenceAsString$I$I(visible.start, visible.end + 1);
if (!this.phgoMatchCase) {
seqString=typeof seqString.toUpperCase$ == "function" ? seqString.toUpperCase$() : seqString.toUpperCase();
}var match=C$.findPhgoMotifMatch$OA$S$I$I(searchPattern, seqString, 0, seqString.length);
if (match != null) {
return this.recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I(seq, visible.start + match[0], visible.start + match[1]);
} else {
this.columnIndex=visible.end + 1;
}return false;
});

Clazz.newMeth(C$, 'recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I', function (seq, startColumn, endColumn) {
this.columnIndex=endColumn + 1;
var matches=this.searchResults.__phgoColumnMatches || (this.searchResults.__phgoColumnMatches=[]);
for (var i=0; i < matches.length; i++) {
var existing=matches[i];
if (existing.sequence === seq && existing.start == startColumn && existing.end == endColumn) {
return false;
}}
matches.push({sequence:seq, start:startColumn, end:endColumn});
var seqStart=Number(seq.getStart$());
var seqEnd=Number(seq.getEnd$());
var firstResidueColumn=-1;
var lastResidueColumn=-1;
var matchedText=seq.getSequenceAsString$I$I(startColumn, endColumn + 1);
for (var columnOffset=0; columnOffset < matchedText.length; columnOffset++) {
if (!C$.phgoGapCharacter$C(matchedText.charAt(columnOffset))) {
if (firstResidueColumn < 0) firstResidueColumn=startColumn + columnOffset;
lastResidueColumn=startColumn + columnOffset;
}}
var nativeStart=Number(seq.findPosition$I(firstResidueColumn < 0 ? startColumn : firstResidueColumn));
var nativeEnd=Number(seq.findPosition$I(lastResidueColumn < 0 ? startColumn : lastResidueColumn));
if (Number.isFinite(seqStart) && Number.isFinite(seqEnd) && seqStart <= seqEnd) {
nativeStart=Math.max(seqStart, Math.min(seqEnd, nativeStart));
nativeEnd=Math.max(seqStart, Math.min(seqEnd, nativeEnd));
}
if (!Number.isFinite(nativeStart)) nativeStart=Number.isFinite(seqStart) ? seqStart : 0;
if (!Number.isFinite(nativeEnd)) nativeEnd=nativeStart;
this.searchResults.addResult$jalview_datamodel_SequenceI$I$I(seq, nativeStart, nativeEnd);
if (firstResidueColumn >= 0) {
var featureable=this.searchResults.__phgoFeatureableNativeRanges || (this.searchResults.__phgoFeatureableNativeRanges=[]);
featureable.push({sequence:seq, start:nativeStart, end:nativeEnd});
} else {
var gapOnly=this.searchResults.__phgoGapOnlyNativeRanges || (this.searchResults.__phgoGapOnlyNativeRanges=[]);
gapOnly.push({sequence:seq, start:nativeStart, end:nativeEnd});
}
return true;
});

Clazz.newMeth(C$, 'doNonMotifSearches$jalview_datamodel_SequenceI$S$com_stevesoft_pat_Regex$Z', function (seq, searchString, searchPattern, includeDescription) {
this.columnIndex=0;
if (this.searchForResidueNumber$jalview_datamodel_SequenceI$S(seq, searchString)) {
return true;
}if (this.searchSequenceName$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(seq, this.phgoIdPattern)) {
return true;
}if (includeDescription && this.searchSequenceDescription$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(seq, this.phgoIdPattern) ) {
return true;
}return false;
});

Clazz.newMeth(C$, 'searchSequenceDescription$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex', function (seq, searchPattern) {
var desc=seq.getDescription$();
if (desc != null  && searchPattern.search$S(desc)  && !this.idMatches.contains$O(seq) ) {
this.idMatches.addElement$TE(seq);
return true;
}return false;
});

Clazz.newMeth(C$, 'searchSequenceName$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex', function (seq, searchPattern) {
if (searchPattern.search$S(seq.getName$()) && !this.idMatches.contains$O(seq) ) {
this.idMatches.addElement$TE(seq);
return true;
}return false;
});

Clazz.newMeth(C$, 'searchForResidueNumber$jalview_datamodel_SequenceI$S', function (seq, searchString) {
try {
var res=Integer.parseInt$S(searchString);
if (seq.getStart$() <= res && seq.getEnd$() >= res ) {
this.searchResults.addResult$jalview_datamodel_SequenceI$I$I(seq, res, res);
return true;
}} catch (ex) {
if (Clazz.exceptionOf(ex,"NumberFormatException")){
} else {
throw ex;
}
}
return false;
});

Clazz.newMeth(C$, 'getIdMatches$', function () {
return this.idMatches;
});

Clazz.newMeth(C$, 'setPhgoTailMapMode$Z', function (enabled) {
this.phgoTailMapMode=enabled === true;
});

Clazz.newMeth(C$, 'phgoTailRangeIsSearchable$jalview_api_AlignViewportI$jalview_datamodel_SequenceI$I$I', function (viewport, seq, start, end) {
var selection=viewport.getSelectionGroup$();
if (selection != null  && selection.getSize$() < 1 ) selection=null;
if (selection != null ) {
if (!selection.contains$jalview_datamodel_SequenceI(seq)) return false;
if (start < selection.getStartRes$() || end > selection.getEndRes$()) return false;
}
var alignment=viewport.getAlignment$();
var hiddenColumns=alignment.getHiddenColumns$ ? alignment.getHiddenColumns$() : null;
if (hiddenColumns == null) return true;
var visibleRegions=hiddenColumns.getVisContigsIterator$I$I$Z(start, alignment.getWidth$(), false);
while (visibleRegions.hasNext$()) {
var visible=visibleRegions.next$();
if (visible[0] > start) return false;
if (visible[0] <= start && visible[1] >= end) return true;
}
return false;
}, 1);

Clazz.newMeth(C$, 'buildPhgoTailAnchors$jalview_api_AlignViewportI$OA$Z', function (viewport, tokens, matchCase) {
var anchors=[];
if (viewport == null) return anchors;
var alignment=viewport.getAlignment$();
if (alignment == null || tokens == null || tokens.length == 0) return anchors;
var width=alignment.getWidth$();
var selection=viewport.getSelectionGroup$();
if (selection != null  && selection.getSize$() < 1 ) selection=null;
var sources=[];
for (var sequenceIndex=0; sequenceIndex < alignment.getHeight$(); sequenceIndex++) {
var sourceSequence=alignment.getSequenceAt$I(sequenceIndex);
if (selection != null  && !selection.contains$jalview_datamodel_SequenceI(sourceSequence) ) continue;
var sourceText=sourceSequence.getSequenceAsString$I$I(0, Math.min(width, sourceSequence.getLength$()));
if (!matchCase) sourceText=typeof sourceText.toUpperCase$ == "function" ? sourceText.toUpperCase$() : sourceText.toUpperCase();
sources.push({sequence:sourceSequence, text:sourceText});
}
var matchedPrefixLength=0;
for (var candidateLength=tokens.length; candidateLength > 0; candidateLength--) {
var prefix=tokens.slice(0, candidateLength);
var candidateAnchors=[];
var seenHeads={};
for (var sourceIndex=0; sourceIndex < sources.length; sourceIndex++) {
var seq=sources[sourceIndex].sequence;
var text=sources[sourceIndex].text;
var cursor=0;
while (cursor < text.length) {
var found=C$.findPhgoMotifMatch$OA$S$I$I(prefix, text, cursor, text.length);
if (found == null) break;
var fullStart=found[0];
var fullEnd=fullStart + tokens.length - 1;
var headKey="$" + fullStart;
if (fullStart >= 0 && fullEnd < width && seenHeads[headKey] !== true && C$.phgoTailRangeIsSearchable$jalview_api_AlignViewportI$jalview_datamodel_SequenceI$I$I(viewport, seq, fullStart, fullEnd)) {
seenHeads[headKey]=true;
candidateAnchors.push({start:fullStart, end:fullEnd, prefixLength:candidateLength, sourceSequenceIndex:sourceIndex});
}
cursor=found[0] + 1;
}
}
if (candidateAnchors.length > 0) {
anchors=candidateAnchors;
matchedPrefixLength=candidateLength;
break;
}
}
if (matchedPrefixLength < 1) return anchors;
anchors.sort(function (a, b) { return a.start - b.start || a.end - b.end || a.sourceSequenceIndex - b.sourceSequenceIndex; });
var kept=[];
for (var i=0; i < anchors.length; i++) {
if (kept.length > 0 && anchors[i].start <= kept[kept.length - 1].end) continue;
kept.push(anchors[i]);
}
return kept;
}, 1);

// Globally align the complete motif tokens to one fixed-width mapped range.
// The anchor range never changes; alignment gaps only decide which individual
// query token corresponds to which existing alignment column. This lets a
// conserved tail such as WELPDGMK remain selectable after an internal indel.
Clazz.newMeth(C$, 'mapPhgoTokensToColumns$OA$S', function (tokens, text) {
var mapped=[];
if (tokens == null || tokens.length == 0 || text == null || text.length == 0) return mapped;
var tokenCount=tokens.length;
var columnCount=text.length;
var stride=columnCount + 1;
var trace=new Array((tokenCount + 1) * stride);
var previous=new Array(stride);
var current=new Array(stride);
var gapPenalty=-2;
var matchScore=3;
var mismatchScore=-1;
previous[0]=0;
for (var columnIndex=1; columnIndex <= columnCount; columnIndex++) {
previous[columnIndex]=previous[columnIndex - 1] + gapPenalty;
trace[columnIndex]=2;
}
for (var tokenIndex=1; tokenIndex <= tokenCount; tokenIndex++) {
current[0]=previous[0] + gapPenalty;
trace[tokenIndex * stride]=1;
for (var columnIndex=1; columnIndex <= columnCount; columnIndex++) {
var isMatch=C$.matchPhgoTokenAt$O$S$I(tokens[tokenIndex - 1], text, columnIndex - 1);
var diagonal=previous[columnIndex - 1] + (isMatch ? matchScore : mismatchScore);
var skipToken=previous[columnIndex] + gapPenalty;
var skipColumn=current[columnIndex - 1] + gapPenalty;
var direction=0;
var score=diagonal;
if (skipToken > score) { score=skipToken; direction=1; }
if (skipColumn > score) { score=skipColumn; direction=2; }
current[columnIndex]=score;
trace[tokenIndex * stride + columnIndex]=direction;
}
var swap=previous;
previous=current;
current=swap;
}
var tokenIndex=tokenCount;
var columnIndex=columnCount;
while (tokenIndex > 0 || columnIndex > 0) {
var direction=trace[tokenIndex * stride + columnIndex];
if (tokenIndex > 0 && columnIndex > 0 && direction == 0) {
if (C$.matchPhgoTokenAt$O$S$I(tokens[tokenIndex - 1], text, columnIndex - 1)) mapped.push(columnIndex - 1);
tokenIndex--;
columnIndex--;
} else if (tokenIndex > 0 && (columnIndex == 0 || direction == 1)) {
tokenIndex--;
} else {
columnIndex--;
}
}
mapped.reverse();
return mapped;
}, 1);

Clazz.newMeth(C$, 'mapPhgoAnchoredTokensToColumns$OA$S$I', function (tokens, text, prefixLength) {
var mapped=[];
if (tokens == null || text == null) return mapped;
var fixedLength=Math.max(0, Math.min(prefixLength, tokens.length, text.length));
for (var fixedIndex=0; fixedIndex < fixedLength; fixedIndex++) {
if (C$.matchPhgoTokenAt$O$S$I(tokens[fixedIndex], text, fixedIndex)) mapped.push(fixedIndex);
}
var tailTokens=tokens.slice(fixedLength);
var tailText=text.substring(fixedLength);
var tailMapped=C$.mapPhgoTokensToColumns$OA$S(tailTokens, tailText);
for (var tailIndex=0; tailIndex < tailMapped.length; tailIndex++) mapped.push(fixedLength + tailMapped[tailIndex]);
return mapped;
}, 1);

Clazz.newMeth(C$, 'buildPhgoTailMappedColumns$jalview_api_AlignViewportI$OA$OA$Z', function (viewport, anchors, tokens, matchCase) {
var rows=[];
if (viewport == null || anchors == null || anchors.length == 0 || tokens == null || tokens.length == 0) return rows;
var alignment=viewport.getAlignment$();
if (alignment == null) return rows;
var selection=viewport.getSelectionGroup$();
if (selection != null  && selection.getSize$() < 1 ) selection=null;
for (var sequenceIndex=0; sequenceIndex < alignment.getHeight$(); sequenceIndex++) {
var sequence=alignment.getSequenceAt$I(sequenceIndex);
if (selection != null  && !selection.contains$jalview_datamodel_SequenceI(sequence) ) continue;
var columns=[];
for (var anchorIndex=0; anchorIndex < anchors.length; anchorIndex++) {
var anchor=anchors[anchorIndex];
var rangeText=sequence.getSequenceAsString$I$I(anchor.start, Math.min(anchor.end + 1, sequence.getLength$()));
if (!matchCase) rangeText=typeof rangeText.toUpperCase$ == "function" ? rangeText.toUpperCase$() : rangeText.toUpperCase();
var relativeColumns=C$.mapPhgoAnchoredTokensToColumns$OA$S$I(tokens, rangeText, anchor.prefixLength || 0);
for (var relativeIndex=0; relativeIndex < relativeColumns.length; relativeIndex++) columns.push(anchor.start + relativeColumns[relativeIndex]);
}
rows.push({sequence:sequence, columns:columns});
}
return rows;
}, 1);

// Compatibility helper for callers that only have an alignment. The live Finder
// path uses the viewport-aware overload above so selections and hidden columns
// participate in prefix discovery.
Clazz.newMeth(C$, 'buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z', function (alignment, tokens, matchCase) {
if (alignment == null) return [];
var viewport={
getAlignment$:function () { return alignment; },
getSelectionGroup$:function () { return null; }
};
return C$.buildPhgoTailAnchors$jalview_api_AlignViewportI$OA$Z(viewport, tokens, matchCase);
}, 1);

// PHgo MSA motif syntax is deliberately parsed without a regular expression.
// X is one non-gap residue. '-' is one alignment gap (including Jalview's '.'
// gap form). Bracketed candidates may use slashes, e.g. (N/S/G), or compact
// one-character alternatives, e.g. (NSG). ASCII and full-width punctuation
// are accepted. Matching runs against alignment columns so gap-containing
// motifs retain exact highlight coordinates.
Clazz.newMeth(C$, 'phgoGapCharacter$C', function (value) {
return value == "-" || value == "." || value == " " || value == "\t";
}, 1);

Clazz.newMeth(C$, 'phgoCandidateTokens$S', function (value) {
var tokens=[];
for (var i=0; i < value.length; i++) {
var c=value.charAt(i);
if (c == "X" || c == "x") tokens.push({kind:"any"});
else if (c == "-" || c == "－" || c == "−") tokens.push({kind:"gap"});
else tokens.push({kind:"literal", value:c});
}
return tokens;
}, 1);

Clazz.newMeth(C$, 'parsePhgoMotif$S', function (input) {
var source=String(input == null ? "" : input);
var tokens=[];
var i=0;
while (i < source.length) {
var c=source.charAt(i);
if (c == "X" || c == "x") { tokens.push({kind:"any"}); i++; continue; }
if (c == "-" || c == "－" || c == "−") { tokens.push({kind:"gap"}); i++; continue; }
var close=null;
if (c == "[") close="]";
else if (c == "【") close="】";
else if (c == "［") close="］";
else if (c == "(") close=")";
else if (c == "（") close="）";
if (close != null) {
var end=source.indexOf(close, i + 1);
if (end >= 0) {
var body=source.substring(i + 1, end).replace(/[／、]/g, "/");
var rawCandidates=body.indexOf("/") >= 0 ? body.split("/") : body.split("");
var candidates=[];
for (var partIndex=0; partIndex < rawCandidates.length; partIndex++) {
var candidate=String(rawCandidates[partIndex]).replace(/^\s+|\s+$/g, "");
if (candidate.length > 0) candidates.push(C$.phgoCandidateTokens$S(candidate));
}
if (candidates.length > 0) {
tokens.push({kind:"alternatives", candidates:candidates});
i=end + 1;
continue;
}
}
}
tokens.push({kind:"literal", value:c});
i++;
}
return tokens;
}, 1);

Clazz.newMeth(C$, 'matchPhgoCandidate$OA$S$I', function (tokens, text, start) {
var position=start;
for (var i=0; i < tokens.length; i++) {
if (position >= text.length) return -1;
var token=tokens[i];
var current=text.charAt(position);
if (token.kind == "any") {
if (C$.phgoGapCharacter$C(current)) return -1;
} else if (token.kind == "gap") {
if (!C$.phgoGapCharacter$C(current)) return -1;
} else if (current != token.value) {
return -1;
}
position++;
}
return position;
}, 1);

Clazz.newMeth(C$, 'matchPhgoTokenAt$O$S$I', function (token, text, start) {
if (token == null || text == null || start < 0 || start >= text.length) return false;
if (token.kind == "alternatives") {
for (var i=0; i < token.candidates.length; i++) {
if (C$.matchPhgoCandidate$OA$S$I(token.candidates[i], text, start) == start + 1) return true;
}
return false;
}
return C$.matchPhgoCandidate$OA$S$I([token], text, start) == start + 1;
}, 1);

Clazz.newMeth(C$, 'matchPhgoMotifFrom$OA$S$I$I', function (tokens, text, tokenIndex, position) {
if (tokenIndex >= tokens.length) return position;
if (position >= text.length) return -1;
var token=tokens[tokenIndex];
if (token.kind == "alternatives") {
for (var i=0; i < token.candidates.length; i++) {
var next=C$.matchPhgoCandidate$OA$S$I(token.candidates[i], text, position);
if (next < 0) continue;
var alternativeEnd=C$.matchPhgoMotifFrom$OA$S$I$I(tokens, text, tokenIndex + 1, next);
if (alternativeEnd >= 0) return alternativeEnd;
}
return -1;
}
var nextPosition=C$.matchPhgoCandidate$OA$S$I([token], text, position);
return nextPosition < 0 ? -1 : C$.matchPhgoMotifFrom$OA$S$I$I(tokens, text, tokenIndex + 1, nextPosition);
}, 1);

Clazz.newMeth(C$, 'findPhgoMotifMatch$OA$S$I$I', function (tokens, text, start, endExclusive) {
if (tokens == null || tokens.length == 0 || text == null) return null;
var first=Math.max(0, start);
var limit=Math.min(text.length, endExclusive);
for (var position=first; position < limit; position++) {
var matchEnd=C$.matchPhgoMotifFrom$OA$S$I$I(tokens, text, 0, position);
if (matchEnd > position && matchEnd <= limit) return [position, matchEnd - 1];
}
return null;
}, 1);

Clazz.newMeth(C$, 'installPhgoColumnResultBridge$jalview_datamodel_SearchResults', function (results) {
if (results == null || results.__phgoColumnResultBridge) return;
results.__phgoColumnResultBridge=true;
results.__phgoColumnMatches=[];
results.__phgoCanCreateFeatureForMatch=function (match) {
var featureable=this.__phgoFeatureableNativeRanges || [];
var sequence=match.getSequence$ ? match.getSequence$() : match.sequence;
var start=match.getStart$ ? match.getStart$() : match.start;
var end=match.getEnd$ ? match.getEnd$() : match.end;
for (var i=0; i < featureable.length; i++) {
var range=featureable[i];
if (range.sequence === sequence && range.start == start && range.end == end) return true;
}
var gapOnly=this.__phgoGapOnlyNativeRanges || [];
for (var i=0; i < gapOnly.length; i++) {
var range=gapOnly[i];
if (range.sequence === sequence && range.start == start && range.end == end) return false;
}
return true;
};
results.__phgoNativeGetSize=results.getSize$;
results.getSize$=function () {
var direct=this.__phgoColumnMatches || [];
return direct.length > 0 ? direct.length : (typeof this.__phgoNativeGetSize == "function" ? this.__phgoNativeGetSize.call(this) : 0);
};
results.__phgoNativeIsEmpty=results.isEmpty$;
results.isEmpty$=function () {
var direct=this.__phgoColumnMatches || [];
return direct.length > 0 ? false : (typeof this.__phgoNativeIsEmpty == "function" ? this.__phgoNativeIsEmpty.call(this) : true);
};
var method="getResults$jalview_datamodel_SequenceI$I$I";
var base=results[method];
results[method]=function (sequence, start, end) {
var direct=this.__phgoColumnMatches || [];
var applies=false;
var columns=[];
var dataset=null;
try { dataset=sequence.getDatasetSequence$(); } catch (_error) {}
for (var i=0; i < direct.length; i++) {
var match=direct[i];
if (match.sequence !== sequence && match.sequence !== dataset) continue;
applies=true;
var from=Math.max(start, match.start);
var to=Math.min(end, match.end);
if (from <= to) columns.push(from, to);
}
if (applies) return columns.length > 0 ? columns : null;
return typeof base == "function" ? base.call(this, sequence, start, end) : null;
};
}, 1);

Clazz.newMeth(C$, 'getSearchResults$', function () {
return this.searchResults;
});

Clazz.newMeth(C$);
})();
;Clazz.setTVer('3.2.4.07');//Created 2019-05-24 12:54:05 Java2ScriptVisitor version 3.2.4.07 net.sf.j2s.core.jar version 3.2.4.07
