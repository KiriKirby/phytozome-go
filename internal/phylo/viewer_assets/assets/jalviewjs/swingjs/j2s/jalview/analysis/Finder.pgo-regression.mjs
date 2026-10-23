import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { execFileSync } from "node:child_process";

const snapshotPath = process.argv[2];
if (!snapshotPath) {
  throw new Error("usage: node Finder.pgo-regression.mjs <snapshot.pgo>");
}

const finderSource = fs.readFileSync(new URL("./Finder.js", import.meta.url), "utf8");
const clazz = {
  lastFinder: null,
  newPackage: () => ({}),
  newClass: () => {
    clazz.lastFinder = function Finder() {};
    return clazz.lastFinder;
  },
  load: () => null,
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
vm.runInContext(finderSource, context, { filename: "Finder.js" });

const Finder = clazz.lastFinder;
const parse = Finder.parsePhgoMotif$S;
const matchToken = Finder.matchPhgoTokenAt$O$S$I;
const fastaText = execFileSync(
  "tar",
  ["-xOf", snapshotPath, "artifacts/tree/1/20261014_200307_000/aligned.fasta"],
  { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 }
);

const records = [];
let current = null;
for (const rawLine of fastaText.split(/\r?\n/)) {
  const line = rawLine.trim();
  if (!line) continue;
  if (line.startsWith(">")) {
    current = { name: line.slice(1), text: "" };
    records.push(current);
  } else {
    assert.ok(current, "FASTA sequence data must follow a header");
    current.text += line;
  }
}

assert.equal(records.length, 32, "the requested PGO regression alignment must contain 32 rows");
const width = Math.max(...records.map((record) => record.text.length));
assert.equal(width, 634, "the requested PGO regression alignment must contain 634 columns");

const sequences = records.map((record) => ({
  ...record,
  getLength$: () => record.text.length,
  getSequenceAsString$I$I: (start, end) => record.text.slice(start, end)
}));
const alignment = {
  getWidth$: () => width,
  getHeight$: () => sequences.length,
  getSequenceAt$I: (index) => sequences[index]
};

const queries = [
  "PDFKG(N/S/G)NFEFLPFGSGRRSCPGMQLGLYA(I/L)EDLG(V/M/I)AHLLH(D/C)F(N/K)WELP(Y/D)(G/R)MK",
  "KETL(R/H)LHP(A/P)IPLLLHE(T/A)(A/C)ED(V/A)EVS(G/D)YF(I/V)PK(R/K)SR(V/I)M(V/I)NAFAI(G/S/A)RDP(N/K)(S/F)W(S/A)(N/D)",
  "IF(S/A)NR(P/S)(T/V/A)N(I/A)AI(N/S/T/A)YLTYDRSDMAFA(N/H)YGPFWRQMRKI(T/S/C)VMKL(F/L)SR(R/K)RAESW",
  "EFIKILQEFS(R/K)LFGAFNIADFIP(F/G/C)LGWMDP(Q/S)GL(R/N/K/H)(P/T/S)R(M/L)V(N/K/S)AR(S/K/G/C)(S/D/A)LD(N/K)FID(Q/R/K)",
  "(S/R)(R/S)LPLPPGP(K/N/R)GLPIIGN(M/I)LMMDQ(L/F)TH(R/H)GL(A/T)(K/N/Q)LAK(Q/K)YG(G/A)(L/I/G)(F/L/G)(H/I/F)",
  "(A/Y)F(N/R/K)(P/A)(O/L)NVGELIF(T/S)(F/L)TMN(L/I)TY(K/R)AAFG(T/S/C)(S/K)(T/N)(K/E)(M/G)(N/Q)(D/N/A)",
  "NIK(A/G)(I/L)IMDV(M/L)FGGTETVAS(I/A)IEWA(M/l)AEL(L/M)KSP(Q/D/L)DL(K/R)(R/C)(V/I)Q(Q/E/R)E",
  "(S/D/N)ED(V/L/M)DM(S/T)DVFGLTAPRATRL(I/V)A(V/L)P(T/S)(S/P/L)(R/K)L(A/T)C",
  "VV(L/H)(N/D/T)R(H/R/L)VEE(T/S/M/K/C)D(L/F)(E/D)KLTF(L/M/F)(K/R)C",
  "(N/S)(V/I/L)(H/N)(M/V)(M/V/I)(T/G/A)VSS(P/A)(D/E)(V/I)A(R/)(Q/E)VL(Q/K)VH(D/G)",
  "(Q/V/T/F/D/C)(Q/D/A)(H/D/Q/G/A)(D/E/Q)(D/V/H/F)(R/S/P/A)(N/I/S)(P/S)DMVD(E/D)LL(A/V)F(Y/F)(T/S/N)(P/E/H)(D/H/E)(Q/D/S)(Q/S/P/D)(P/T/G/A)(T/S/N/G)(N/V/R/K)(N/T/S/V/A)(S/D/N/W)",
  "(V/T)(R/M/E)DE(I/V)(S/D/A)(S/D/A)(S/C)MV(Q/K)TV(A/K)(A/V)N"
];

const summaries = queries.map((query, queryIndex) => {
  const tokens = parse(query.toUpperCase());
  const anchors = Array.from(
    Finder.buildPhgoTailAnchors$jalview_datamodel_AlignmentI$OA$Z(alignment, tokens, false)
  );
  assert.ok(anchors.length > 0, `query ${queryIndex + 1} must produce at least one mapped range`);
  const prefixLengths = new Set(anchors.map((anchor) => anchor.prefixLength));
  assert.equal(prefixLengths.size, 1, `query ${queryIndex + 1} must fix exactly one prefix length`);

  let matchingCells = 0;
  for (const anchor of anchors) {
    for (const sequence of sequences) {
      const rangeText = sequence.text.slice(anchor.start, anchor.end + 1).toUpperCase();
      matchingCells += Finder.mapPhgoAnchoredTokensToColumns$OA$S$I(tokens, rangeText, anchor.prefixLength).length;
    }
  }
  const possibleCells = anchors.length * sequences.length * tokens.length;
  return {
    query: queryIndex + 1,
    queryLength: tokens.length,
    prefixLength: anchors[0].prefixLength,
    anchors: anchors.map((anchor) => [anchor.start, anchor.end]),
    matchingCells,
    possibleCells,
    density: Number((matchingCells / possibleCells).toFixed(4))
  };
});

assert.equal(summaries[0].anchors.length, 1, "the PDF query must resolve to one horizontal motif location");
assert.deepEqual(summaries[0].anchors[0], [539, 589], "the PDF query must map alignment columns 540-590");
assert.equal(summaries[0].prefixLength, 31, "the PDF query must fix its first successful 31-position prefix");
const firstStart = summaries[0].anchors[0][0];
assert.ok(
  sequences.some((sequence) => sequence.text.slice(firstStart, firstStart + 3).toUpperCase() === "PDF"),
  "the first mapped range must start on the left-side PDF motif"
);
assert.ok(summaries[0].density >= 0.45, "the PDF mapped block must be densely conserved across selected rows");

const firstTokens = parse(queries[0].toUpperCase());
const firstColumnCounts = new Array(firstTokens.length).fill(0);
for (const sequence of sequences) {
  const rangeText = sequence.text.slice(firstStart, firstStart + firstTokens.length).toUpperCase();
  for (const relativeColumn of Finder.mapPhgoAnchoredTokensToColumns$OA$S$I(firstTokens, rangeText, summaries[0].prefixLength)) {
    firstColumnCounts[relativeColumn] += 1;
  }
}
const firstPositionMatches = firstColumnCounts.map((matches, relativeColumn) => ({
  column: firstStart + relativeColumn + 1,
  matches
}));
assert.ok(
  firstPositionMatches.slice(-14).some((position) => position.matches >= 20),
  "the mapped PDF block must retain densely selected cells in its right-hand tail"
);
for (let column = 582; column <= 589; column += 1) {
  const position = firstPositionMatches.find((entry) => entry.column === column);
  assert.ok(position && position.matches > 0, `the conserved WELPDGMK tail column ${column} must remain selected`);
}

console.log(JSON.stringify({ rows: records.length, width, summaries, firstPositionMatches }, null, 2));
