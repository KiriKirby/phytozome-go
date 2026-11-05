import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const source = fs.readFileSync(new URL("./Finder.js", import.meta.url), "utf8");
const clazz = {
  lastFinder: null,
  newPackage: () => ({}),
  newClass: () => {
    clazz.lastFinder = function Finder() {};
    return clazz.lastFinder;
  },
  load: (name) => {
    if (name === "jalview.datamodel.Range") {
      return {
        c$$I$I(start, end) {
          this.start = start;
          this.end = end;
        }
      };
    }
    return null;
  },
  new_: (constructor, args = []) => {
    const value = {};
    const returned = constructor.apply(value, args);
    return returned == null ? value : returned;
  },
  newMeth: (owner, name, implementation, modifier) => {
    if (typeof name !== "string") return null;
    if (modifier === 1) owner[name] = implementation;
    else owner.prototype[name] = implementation;
    return implementation;
  },
  setTVer: () => null
};
const context = { Clazz: clazz };

vm.createContext(context);
vm.runInContext(source, context, { filename: "Finder.js" });

const Finder = clazz.lastFinder;
const parse = Finder?.parsePhgoMotif$S;
const find = Finder?.findPhgoMotifMatch$OA$S$I$I;
const matchToken = Finder?.matchPhgoTokenAt$O$S$I;
assert.equal(typeof parse, "function");
assert.equal(typeof find, "function");
assert.equal(typeof matchToken, "function");

function match(motif, sequence, start = 0, end = sequence.length) {
  const result = find(parse(motif), sequence, start, end);
  return result == null ? null : Array.from(result);
}

const cases = [
  ["(N/S/G)", "AAASAA", [3, 3]],
  ["[N/S/G]", "AAAGAA", [3, 3]],
  ["(NSG)", "AAANAA", [3, 3]],
  ["[NSG]", "AAASAA", [3, 3]],
  ["（N／S／G）", "AAAGAA", [3, 3]],
  ["【N／S／G】", "AAANAA", [3, 3]],
  ["［N／S／G］", "AAASAA", [3, 3]],
  ["（N、S、G）", "AAAGAA", [3, 3]],
  ["AXC", "ZZABCYY", [2, 4]],
  ["AXC", "A-C", null],
  ["ABC-DE", "ZZABC-DEYY", [2, 7]],
  ["ABC-DE", "ZZABC.DEYY", [2, 7]],
  ["ABC－DE", "ZZABC-DEYY", [2, 7]],
  ["ABC−DE", "ZZABC.DEYY", [2, 7]],
  ["(A/-/G)", "-", [0, 0]],
  ["(A-G)", "-", [0, 0]],
  ["[A-G]", "G", [0, 0]],
  ["(A/BC/G)D", "ZZBCD", [2, 4]],
  ["(P/I)PGPX(P/G)XP", "ZZIPGPQGRPYY", [2, 9]],
  ["(P/I)PGPX(P/G)XP", "ZZPPGP-GRPYY", null],
  ["EXXR...P(E/F)R...R", "AAEXXRQQQPERGGGRYY", [2, 15]],
  ["EXXR...P(E/F)R...R", "EXXRPERR", [0, 7]],
  ["ABC...XYZ", "ZZABC---XYZYY", [2, 10]],
  ["EXXR...P(E/F)R...R", "EXXRQQPDRR", null],
];

for (const [motif, sequence, expected] of cases) {
  assert.deepEqual(match(motif, sequence), expected, `${motif} should match ${sequence}`);
}

assert.deepEqual(match("ABC", "xxABCxx", 3), null, "start boundary must be honored");
assert.deepEqual(match("ABC", "xxABCxx", 0, 4), null, "end boundary must be honored");
assert.deepEqual(match("ABC", "xxABCxx"), [2, 4]);
assert.deepEqual(match("ABC", "xxabcxx"), null, "direct matcher remains case-sensitive");
assert.deepEqual(match("ABC", "xxabcxx".toUpperCase()), [2, 4], "Finder case-insensitive path uppercases alignment text");
assert.equal(matchToken(parse("(N/S/G)")[0], "S", 0), true);
assert.equal(matchToken(parse("X")[0], "A", 0), true);
assert.equal(matchToken(parse("X")[0], "-", 0), false);
assert.equal(matchToken(parse("-")[0], ".", 0), true);
assert.equal(parse("A...B")[1].kind, "many");
assert.equal(Finder.phgoHasVariableSpan$OA(parse("A...B")), true);
assert.equal(Finder.phgoHasVariableSpan$OA(parse("AXB")), false);

const results = {
  added: [],
  getSize$: () => 0,
  isEmpty$: () => true,
  addResult$jalview_datamodel_SequenceI$I$I(sequence, start, end) {
    const matchValue = {
      sequence,
      start,
      end,
      getSequence$: () => sequence,
      getStart$: () => start,
      getEnd$: () => end
    };
    this.added.push([sequence, start, end, matchValue]);
    return matchValue;
  },
  getResults$jalview_datamodel_SequenceI$I$I() {
    return [98, 99];
  }
};
Finder.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(results);
const sequence = {
  text: "AB-CD",
  getStart$: () => 1,
  getEnd$: () => 4,
  getSequenceAsString$I$I(start, end) {
    return this.text.slice(start, end);
  },
  findPosition$I(column) {
    let position = 0;
    for (let index = 0; index <= column && index < this.text.length; index += 1) {
      if (this.text[index] !== "-" && this.text[index] !== ".") position += 1;
    }
    return Math.max(1, position);
  },
  getDatasetSequence$: () => null
};
const finder = new Finder();
finder.searchResults = results;
finder.columnIndex = 0;
assert.equal(
  finder.recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I(sequence, 1, 4),
  true
);

const tailRows = [
  makeSequence("XXABCDQQYYABCDZZZ"),
  makeSequence("QQABCDQQYYABCDZZZ")
];
const tailAlignment = {
  getWidth$: () => tailRows[0].text.length,
  getHeight$: () => tailRows.length,
  getSequenceAt$I: (index) => tailRows[index]
};
const tailAnchors = Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
  tailAlignment,
  parse("ABCDEFG"),
  true
);
assert.deepEqual(
  Array.from(tailAnchors, (value) => [value.start, value.end]),
  [[2, 8], [10, 16]],
  "the first successful complete prefix level supplies global heads and extends them to full query width"
);
assert.deepEqual(
  Array.from(tailAnchors, (value) => [value.prefixLength, value.sourceSequenceIndex]),
  [[4, 0], [4, 0]],
  "right trimming must fix the longest successful prefix and identical heads must retain the topmost source row"
);

const overlapRows = [makeSequence("ABCDEFGABCDEFG")];
const overlapAlignment = {
  getWidth$: () => 14,
  getHeight$: () => 1,
  getSequenceAt$I: () => overlapRows[0]
};
const overlapAnchors = Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
  overlapAlignment,
  parse("ABCDEFG"),
  true
);
assert.deepEqual(
  Array.from(overlapAnchors, (value) => [value.start, value.end]),
  [[0, 6], [7, 13]],
  "non-overlapping full-width mapped ranges remain available"
);
assert.deepEqual(
  Array.from(Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
    { getWidth$: () => 12, getHeight$: () => 1, getSequenceAt$I: () => makeSequence("ZZCDEFGCDEFG") },
    parse("ABCDEFG"),
    true
  ), (value) => [value.start, value.end]),
  [],
  "a suffix-only match must not create a head because right trimming retains the left prefix"
);
assert.equal(finder.columnIndex, 5, "next search must continue after the matched columns");
assert.equal(results.getSize$(), 1, "native Finder result counts must reflect exact column matches");
assert.equal(results.isEmpty$(), false);
assert.deepEqual(
  results.added[0].slice(1, 3),
  [2, 4],
  "standard SearchResults must retain the full native residue range used by Create Features"
);
assert.equal(
  results.__phgoCanCreateFeatureForMatch(results.added[0][3]),
  true,
  "a motif containing residues must remain featureable through Jalview's native result list"
);
assert.deepEqual(
  Array.from(results.getResults$jalview_datamodel_SequenceI$I$I(sequence, 0, 9)),
  [1, 4],
  "column bridge must preserve an exact match containing gaps"
);
assert.equal(
  results.getResults$jalview_datamodel_SequenceI$I$I(sequence, 5, 9),
  null,
  "column bridge must clip to the requested visible range"
);
assert.equal(
  finder.recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I(sequence, 1, 4),
  false,
  "duplicate matches must not be recorded"
);

const gapResults = {
  added: [],
  getSize$: () => 0,
  isEmpty$: () => true,
  addResult$jalview_datamodel_SequenceI$I$I(sequenceValue, start, end) {
    const matchValue = {
      sequence: sequenceValue,
      start,
      end,
      getSequence$: () => sequenceValue,
      getStart$: () => start,
      getEnd$: () => end
    };
    this.added.push(matchValue);
    return matchValue;
  }
};
Finder.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(gapResults);
const gapFinder = new Finder();
gapFinder.searchResults = gapResults;
const gapSequence = makeSequence("-");
assert.equal(gapFinder.recordPhgoColumnMatch$jalview_datamodel_SequenceI$I$I(gapSequence, 0, 0), true);
assert.equal(gapResults.getSize$(), 1, "gap-only matches must count as real search results");
assert.equal(gapResults.isEmpty$(), false);
assert.equal(
  gapResults.__phgoCanCreateFeatureForMatch(gapResults.added[0]),
  false,
  "a gap-only match must stay searchable/highlightable without creating a false neighboring-residue feature"
);
const nativeResidueNumberMatch = {
  getSequence$: () => gapSequence,
  getStart$: () => 7,
  getEnd$: () => 7
};
assert.equal(
  gapResults.__phgoCanCreateFeatureForMatch(nativeResidueNumberMatch),
  true,
  "unmarked native results such as Jalview residue-number searches must retain Create Features behavior"
);

const unrelated = { getDatasetSequence$: () => null };
assert.deepEqual(
  Array.from(results.getResults$jalview_datamodel_SequenceI$I$I(unrelated, 0, 9)),
  [98, 99],
  "unrelated sequences must retain native SearchResults behavior"
);

function makeSearchResults() {
  return {
    added: [],
    addResult$jalview_datamodel_SequenceI$I$I(sequenceValue, start, end) {
      const matchValue = {
        sequence: sequenceValue,
        start,
        end,
        getSequence$: () => sequenceValue,
        getStart$: () => start,
        getEnd$: () => end
      };
      this.added.push([sequenceValue, start, end, matchValue]);
      return matchValue;
    },
    getResults$jalview_datamodel_SequenceI$I$I() {
      return null;
    }
  };
}

function makeSequence(text) {
  return {
    text,
    getLength$: () => text.length,
    getSequenceAsString$I$I: (start, end) => text.slice(start, end),
    getStart$: () => 1,
    getEnd$: () => Array.from(text).filter((value) => value !== "-" && value !== ".").length,
    findPosition$I: (column) => {
      let position = 0;
      for (let index = 0; index <= column && index < text.length; index += 1) {
        if (text[index] !== "-" && text[index] !== ".") position += 1;
      }
      return Math.max(1, position);
    },
    getDatasetSequence$: () => null
  };
}

const actualFinder = new Finder();
const actualResults = makeSearchResults();
Finder.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(actualResults);
actualFinder.searchResults = actualResults;
actualFinder.columnIndex = 0;
actualFinder.phgoMatchCase = false;
actualFinder.viewport = {
  getSelectionGroup$: () => null,
  getAlignment$: () => ({
    getWidth$: () => 6,
    getHiddenColumns$: () => ({
      getVisContigsIterator$I$I$Z: (column) => {
        let returned = false;
        return {
          hasNext$: () => !returned && column < 6,
          next$: () => {
            returned = true;
            return [column, 5];
          }
        };
      }
    })
  })
};
const actualSequence = makeSequence("N-S-G-");
const actualPattern = parse("(NSG)-");
const searchVisible = actualFinder.searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex;
assert.equal(searchVisible.call(actualFinder, actualSequence, actualPattern), true);
assert.equal(searchVisible.call(actualFinder, actualSequence, actualPattern), true);
assert.equal(searchVisible.call(actualFinder, actualSequence, actualPattern), true);
assert.deepEqual(
  Array.from(actualResults.__phgoColumnMatches, (value) => [value.start, value.end]),
  [[0, 1], [2, 3], [4, 5]],
  "the real visible-region search path must find every compact alternative plus gap without looping"
);
assert.equal(actualFinder.columnIndex, 6);

const variableFinder = new Finder();
const variableResults = makeSearchResults();
Finder.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(variableResults);
variableFinder.searchResults = variableResults;
variableFinder.columnIndex = 0;
variableFinder.phgoMatchCase = true;
variableFinder.phgoTailMapMode = true;
variableFinder.phgoTailMapActive = false;
variableFinder.phgoTailTokens = parse("EXXR...P(E/F)R...R");
variableFinder.viewport = {
  getSelectionGroup$: () => null,
  getAlignment$: () => ({
    getWidth$: () => 12,
    getHiddenColumns$: () => ({
      getVisContigsIterator$I$I$Z: (column) => {
        let returned = false;
        return {
          hasNext$: () => !returned && column < 12,
          next$: () => {
            returned = true;
            return [column, 11];
          }
        };
      }
    })
  })
};
const variableSequence = makeSequence("EXXRQQPER--R");
assert.equal(
  variableFinder.searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(
    variableSequence,
    variableFinder.phgoTailTokens
  ),
  true,
  "variable-span motifs must use exact search when prefix mapping is enabled"
);
assert.deepEqual(
  Array.from(variableResults.__phgoColumnMatches, (value) => [value.start, value.end]),
  [[0, 11]]
);

const tailSearchResults = makeSearchResults();
Finder.installPhgoColumnResultBridge$jalview_datamodel_SearchResults(tailSearchResults);
const tailFinder = new Finder();
tailFinder.searchResults = tailSearchResults;
tailFinder.columnIndex = 0;
tailFinder.phgoTailMapMode = true;
tailFinder.phgoMatchCase = true;
tailFinder.phgoTailTokens = parse("ABCDEFG");
tailFinder.phgoTailAnchors = [{ start: 2, end: 8 }];
const mappedSequence = makeSequence("ZZABXDEGQZZZ");
tailFinder.phgoTailMappedColumns = [{
  sequence: mappedSequence,
  columns: Array.from(
    Finder.mapPhgoAnchoredTokensToColumns$OA$S$I(tailFinder.phgoTailTokens, mappedSequence.text.slice(2, 9), 2),
    (column) => column + 2
  )
}];
tailFinder.viewport = {
  getSelectionGroup$: () => null,
  getAlignment$: () => ({
    getWidth$: () => 12,
    getHiddenColumns$: () => ({
      getVisContigsIterator$I$I$Z: (column) => {
        let returned = false;
        return {
          hasNext$: () => !returned && column < 12,
          next$: () => {
            returned = true;
            return [column, 11];
          }
        };
      }
    })
  })
};
assert.equal(
  tailFinder.searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(mappedSequence, parse("ABCDEFG")),
  true,
  "mapped comparison must return its first matching character through native SearchResults"
);
while (tailFinder.columnIndex < mappedSequence.getLength$()) {
  tailFinder.searchNextVisibleRegion$jalview_datamodel_SequenceI$com_stevesoft_pat_Regex(mappedSequence, parse("ABCDEFG"));
}
assert.deepEqual(
  Array.from(tailSearchResults.__phgoColumnMatches, (value) => [value.start, value.end]),
  [[2, 2], [3, 3], [5, 5], [6, 6], [7, 7]],
  "tail mapping must select only individually matching character positions, never the whole mapped range"
);

const screenshotQuery = parse("PDFKG(N/S/G)NFEFLPFGSGRR");
const screenshotRow = makeSequence("-PGVPDFKGSNFEFLPFGSGRRSC");
const screenshotAnchors = Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
  { getWidth$: () => screenshotRow.text.length, getHeight$: () => 1, getSequenceAt$I: () => screenshotRow },
  screenshotQuery,
  true
);
assert.deepEqual(
  Array.from(screenshotAnchors, (value) => [value.start, value.end]),
  [[4, 21]],
  "the screenshot-style PDFKG alternative query must find its complete suffix anchor"
);
const suffixOnlyRow = makeSequence("ZZSNFEFLPFGSGRRABCDEFG");
const suffixOnlyAnchors = Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
  { getWidth$: () => suffixOnlyRow.text.length, getHeight$: () => 1, getSequenceAt$I: () => suffixOnlyRow },
  screenshotQuery,
  true
);
assert.deepEqual(
  Array.from(suffixOnlyAnchors, (value) => [value.start, value.end]),
  [],
  "a complete suffix hit alone must not anchor a right-trimmed prefix search"
);

const realPgoQuery = parse("PDFKG(N/S/G)NFEFLPFGSGRRSCPGMQLGLYA(I/L)EDLG(V/M/I)AHLLH(D/C)F(N/K)WELP(Y/D)(G/R)MK");
const realPgoRow = makeSequence("PGVPDFKGSNFEFIPFGSGRRSCPGMQLGLYALDLAVAHLLHCFTWELPDGMKPSELDMTDMFGLTAPRATRLVAVPRKRVVCPL");
const realPgoAnchors = Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(
  { getWidth$: () => realPgoRow.text.length, getHeight$: () => 1, getSequenceAt$I: () => realPgoRow },
  realPgoQuery,
  true
);
assert.deepEqual(
  Array.from(realPgoAnchors, (value) => [value.start, value.end]),
  [[3, 53]],
  "the real PGO regression query must anchor at the left PDF and extend by the complete query width"
);
const realPgoMappedColumns = Array.from(
  Finder.mapPhgoAnchoredTokensToColumns$OA$S$I(realPgoQuery, realPgoRow.text.slice(3, 54), realPgoAnchors[0].prefixLength),
  (column) => column + 3
);
const realPgoTailStart = realPgoRow.text.indexOf("WELPDGMK", 3);
assert.ok(realPgoTailStart >= 0);
assert.deepEqual(
  realPgoMappedColumns.filter((column) => column >= realPgoTailStart && column < realPgoTailStart + 8),
  Array.from({ length: 8 }, (_, index) => realPgoTailStart + index),
  "fixed-width projection must realign after an internal indel and select the complete WELPDGMK tail through K"
);
const realPgoMappedRows = Finder.buildPhgoTailMappedColumns$jalview_api_AlignViewportI$OA$OA$Z(
  {
    getAlignment$: () => ({
      getWidth$: () => realPgoRow.text.length,
      getHeight$: () => 1,
      getSequenceAt$I: () => realPgoRow
    }),
    getSelectionGroup$: () => null
  },
  realPgoAnchors,
  realPgoQuery,
  true
);
assert.deepEqual(
  Array.from(realPgoMappedRows[0].columns),
  realPgoMappedColumns,
  "the live Finder projection cache must use the same indel-aware complete-query mapping"
);

console.log("Finder PHgo motif tests passed");
