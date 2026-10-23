import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const source = fs.readFileSync(new URL("./Finder.js", import.meta.url), "utf8");
let Finder;
let shownEditor = null;

class JavaList {
  constructor() {
    this.values = [];
  }
  add$TE(value) {
    this.values.push(value);
    return true;
  }
  isEmpty$() {
    return this.values.length === 0;
  }
}

const clazz = {
  newPackage: () => ({}),
  newClass: (_owner, name) => {
    const Type = function Type() {};
    if (name === "Finder") Finder = Type;
    return Type;
  },
  load(name) {
    if (name === "java.util.ArrayList") return JavaList;
    if (name === "jalview.datamodel.SequenceFeature") {
      return {
        c$$S$S$I$I$S(type, description, start, end, group) {
          this.type = type;
          this.description = description;
          this.start = start;
          this.end = end;
          this.group = group;
        }
      };
    }
    if (name === "jalview.gui.FeatureEditor") {
      return {
        c$$jalview_gui_AlignmentPanel$java_util_List$java_util_List$Z(panel, sequences, features, editable) {
          shownEditor = { panel, sequences: sequences.values, features: features.values, editable };
          this.showDialog$ = () => {
            shownEditor.shown = true;
          };
        }
      };
    }
    return null;
  },
  newMeth(owner, name, implementation, modifier) {
    if (typeof name !== "string") return null;
    if (modifier === 1) owner[name] = implementation;
    else owner.prototype[name] = implementation;
    return implementation;
  },
  new_(constructor, args = []) {
    if (constructor === JavaList) return new JavaList();
    const value = {};
    const returned = constructor.apply(value, args);
    return returned == null ? value : returned;
  },
  setTVer: () => null,
  exceptionOf: () => false,
  instanceOf: () => false
};

vm.createContext({ Clazz: clazz });
vm.runInContext(source, vm.createContext({ Clazz: clazz }), { filename: "gui/Finder.js" });

function iterator(values) {
  let index = 0;
  return {
    hasNext$: () => index < values.length,
    next$: () => values[index++]
  };
}

function nativeMatch(sequence, start, end) {
  return {
    getSequence$: () => sequence,
    getStart$: () => start,
    getEnd$: () => end
  };
}

const datasetA = { id: "dataset-a" };
const datasetB = { id: "dataset-b" };
const sequenceA = { getDatasetSequence$: () => datasetA };
const sequenceB = { getDatasetSequence$: () => datasetB };
const featureableA = nativeMatch(sequenceA, 12, 16);
const gapOnly = nativeMatch(sequenceA, 16, 16);
const featureableB = nativeMatch(sequenceB, 3, 9);
const matches = [featureableA, gapOnly, featureableB];

const finder = new Finder();
finder.ap = { id: "alignment-panel" };
finder.searchBox = { getUserInput$: () => "(N/S/G)-X" };
finder.searchResults = {
  isEmpty$: () => false,
  getResults$: () => ({ iterator$: () => iterator(matches) }),
  __phgoCanCreateFeatureForMatch: (match) => match !== gapOnly
};

assert.equal(finder.hasFeatureableSearchResults$(), true);
finder.createFeatures_actionPerformed$();
assert.equal(shownEditor?.shown, true, "Create Features must still invoke Jalview's native FeatureEditor");
assert.deepEqual(shownEditor.sequences, [datasetA, datasetB]);
assert.deepEqual(
  shownEditor.features.map(({ type, start, end, group }) => ({ type, start, end, group })),
  [
    { type: "(N/S/G)-X", start: 12, end: 16, group: "Search Results" },
    { type: "(N/S/G)-X", start: 3, end: 9, group: "Search Results" }
  ],
  "all featureable matches must preserve their complete native residue ranges"
);

shownEditor = null;
finder.searchResults = {
  isEmpty$: () => false,
  getResults$: () => ({ iterator$: () => iterator([gapOnly]) }),
  __phgoCanCreateFeatureForMatch: () => false
};
assert.equal(finder.hasFeatureableSearchResults$(), false);
finder.createFeatures_actionPerformed$();
assert.equal(shownEditor, null, "gap-only results cannot be represented as biological residue features");

console.log("Jalview GUI Finder feature integration tests passed");
