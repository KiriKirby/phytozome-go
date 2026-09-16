import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const source = fs.readFileSync(new URL("./index.js", import.meta.url), "utf8");
const context = {
  window: {},
  document: {},
  console,
  fetch: async () => {
    throw new Error("fetch is not available in parser tests");
  },
  URL,
  Blob,
  Image: class {}
};
context.window.console = console;
context.window.setTimeout = setTimeout;
context.window.clearTimeout = clearTimeout;
vm.createContext(context);
vm.runInContext(source, context, { filename: "index.js" });

const api = context.window.PHGOmsaexpor;

function sameJSON(actual, expected) {
  assert.equal(JSON.stringify(actual), JSON.stringify(expected));
}

const rows = [
  { taxon_id: "a", display_name: "Alpha", display_prefix: "[1,4]", canvas_item_index: 1, canvas_row: 4, index: 0, state: "green", sequence: "ABCDEFGHIJ".repeat(30) },
  { taxon_id: "b", display_name: "Beta", display_prefix: "[1,3]", canvas_item_index: 1, canvas_row: 3, index: 1, state: "green", sequence: "AB-D F*HIJ".repeat(30) },
  { taxon_id: "c", display_name: "Gamma", display_prefix: "[1,6]", canvas_item_index: 1, canvas_row: 6, index: 2, state: "yellow", sequence: "ABCDEFGHIJ".repeat(30) },
  { taxon_id: "d", display_name: "Delta", display_prefix: "[1,5]", canvas_item_index: 1, canvas_row: 5, index: 3, state: "red", sequence: "ABCDEFGHIJ".repeat(30) }
];
const model = {
  rows,
  alignmentWidth: 300,
  groups: [
    { name: "manual", start: 10, end: 39, sequences: ["Alpha", "Beta"] }
  ],
  features: [
    { taxon_id: "a", type: "domain", begin: 12, end: 16 },
    { sequence_index: 1, type: "motif", begin: 12, end: 14 }
  ]
};

{
  const plan = api.parseAdvancedLayout(">1,4/1,3\\10,100", model);
  assert.equal(plan.blocks.length, 1);
  assert.equal(plan.blocks[0].columnStartBoundary, 10);
  assert.equal(plan.blocks[0].columnEndBoundary, 100);
  assert.equal(plan.blocks[0].visibleColumnCount, 90);
}

{
  const plan = api.parseAdvancedLayout(">1,1/1,2/1,3/1,4/1,5/1,6\\10,100", {
    alignmentWidth: 300,
    rows: [
      { taxon_id: "one", display_prefix: "[1,1]", sequence: "A".repeat(300) },
      { taxon_id: "two", display_prefix: "[1,2]", sequence: "A".repeat(300) },
      { taxon_id: "three", display_prefix: "[1,3]", sequence: "A".repeat(300) },
      { taxon_id: "four", display_prefix: "[1,4]", sequence: "A".repeat(300) },
      { taxon_id: "five", display_prefix: "[1,5]", sequence: "A".repeat(300) },
      { taxon_id: "six", display_prefix: "[1,6]", sequence: "A".repeat(300) }
    ]
  });
  assert.equal(plan.blocks[0].rows.length, 6);
}

// Canvas stores row indexes internally as zero-based values while the MSA
// labels and the documented DSL are one-based.  A live export model contains
// both; the visible prefix must win so the script users see is usable verbatim.
{
  const plan = api.parseAdvancedLayout(">1,1/1,2\\0,10", {
    alignmentWidth: 10,
    rows: [
      { taxon_id: "first", display_prefix: "[1,1]", canvas_item_index: 0, canvas_row: 0, sequence: "A".repeat(10) },
      { taxon_id: "second", display_prefix: "[1,2]", canvas_item_index: 0, canvas_row: 1, sequence: "A".repeat(10) }
    ]
  });
  assert.equal(plan.blocks[0].rows[0].taxon_id, "first");
  assert.equal(plan.blocks[0].rows[1].taxon_id, "second");
}

{
  const plan = api.parseAdvancedLayout(">~\\10,100/~,~,~", model);
  assert.equal(plan.blocks.length, 3);
  assert.equal(plan.blocks[0].rows.length, rows.length);
  sameJSON(plan.blocks.map((b) => [b.columnStartBoundary, b.columnEndBoundary]), [[10, 40], [40, 70], [70, 100]]);
  assert.throws(() => api.parseAdvancedLayout(">~/1,3\\10,100", model), /all-row shorthand must be the only item token/);
}

{
  const plan = api.parseAdvancedLayout(">1,4/1,3\\10,100/30,20,40", model);
  sameJSON(plan.blocks.map((b) => [b.columnStartBoundary, b.columnEndBoundary]), [[10, 40], [40, 60], [60, 100]]);
  assert.equal(plan.blocks[0].alignmentWidthForNumbering, 40);
}

{
  const plan = api.parseAdvancedLayout(">1,4/1,3\\10,100/~,30,~", model);
  sameJSON(plan.blocks.map((b) => [b.columnStartBoundary, b.columnEndBoundary]), [[10, 40], [40, 70], [70, 100]]);
}

{
  const plan = api.parseAdvancedLayout(">1,4/1,3\\10,~\n>1,6\\~,10\n>1,4\\~,~", model);
  sameJSON(plan.blocks.map((b) => [b.columnStartBoundary, b.columnEndBoundary]), [[10, 300], [0, 10], [0, 300]]);
}

{
  assert.throws(() => api.parseAdvancedLayout(">1,4/1,4\\10,100", model), /duplicate PHgo row coordinate/);
  assert.throws(() => api.parseAdvancedLayout(">1,4 /1,3\\10,100", model), /internal spaces/);
  assert.throws(() => api.parseAdvancedLayout("# comment", model), /comments are not supported/);
  assert.throws(() => api.parseAdvancedLayout(">1,4/1,3\\10,100/30,30", model), /must equal range length/);
  assert.throws(() => api.parseAdvancedLayout(">1,4/1,3\\10,100/37,~,~", model), /cannot be divided/);
  assert.throws(() => api.parseAdvancedLayout(">1,9\\10,100", model), /was not found/);
}

{
  const plan = api.buildAutomaticLayout(model);
  assert.equal(plan.blocks.length, 1);
  assert.equal(plan.blocks[0].rows.length, 4);
  sameJSON(plan.blocks.map((b) => [b.columnStartBoundary, b.columnEndBoundary]), [[0, 300]]);
}

{
  assert.throws(() => api.resolveLayout(api.normalizeSettings(), { rows: [], alignmentWidth: 10 }), /No MSA rows to export/);
  assert.throws(() => api.resolveLayout(api.normalizeSettings(), { rows, alignmentWidth: 0 }), /No aligned MSA columns to export/);
}

{
  const rowsFromFasta = api.buildRows(
    { aligned_fasta: ">One\nAAAA\n>Two\nAA--\n", metadata: {} },
    { rows: [] },
    {}
  );
  assert.equal(rowsFromFasta.length, 2);
  assert.equal(rowsFromFasta[0].display_name, "One");
  assert.equal(rowsFromFasta[1].sequence, "AA--");
}

{
  const rowsFromState = api.buildRows(
    { metadata: {} },
    { rows: [] },
    { sequences: [{ taxon_id: "s1", display_name: "StateOne", sequence: "ACD" }] }
  );
  assert.equal(rowsFromState.length, 1);
  assert.equal(rowsFromState[0].display_name, "StateOne");
  assert.equal(rowsFromState[0].sequence, "ACD");
}

{
  const rowsFromLiveStateOrder = api.buildRows(
    { metadata: { records: [
      { taxon_id: "a", display_name: "PayloadA", canvas_item_index: 1, canvas_row: 1 },
      { taxon_id: "b", display_name: "PayloadB", canvas_item_index: 1, canvas_row: 2 }
    ] } },
    { rows: [] },
    { sequences: [
      { taxon_id: "b", display_name: "LiveB", sequence: "BBB" },
      { taxon_id: "a", display_name: "LiveA", sequence: "AAA" }
    ] }
  );
  sameJSON(rowsFromLiveStateOrder.map((row) => row.taxon_id), ["b", "a"]);
  sameJSON(rowsFromLiveStateOrder.map((row) => row.sequence), ["BBB", "AAA"]);
  sameJSON(rowsFromLiveStateOrder.map((row) => row.canvas_row), [2, 1]);
}

{
  const emptyModel = { rows: [], alignmentWidth: 10, groups: [], features: [] };
  const layout = { blocks: [{ rows: [], columnStartBoundary: 0, columnEndBoundary: 10, visibleColumnCount: 10, alignmentWidthForNumbering: 10 }] };
  assert.throws(() => api.renderScene(emptyModel, api.normalizeSettings(), layout, null), /requires the Jalview native render bridge/);
}

{
  const layout = api.buildAutomaticLayout(model);
  const scene = api.renderScene(model, api.normalizeSettings(), layout, {
    renderMSAExportScene(settings, receivedLayout) {
      assert.equal(settings.scale, 2);
      assert.equal(receivedLayout, layout);
      return { svg: "<svg data-msaexpor=\"1\" data-msaexpor-renderer=\"jalview-native\"></svg>", width: 12, height: 8, source: "jalview-native" };
    }
  });
  assert.equal(scene.source, "jalview-native");
  assert.match(scene.svg, /jalview-native/);
}

{
  const settings = api.normalizeSettings({
    format: "pdf",
    scale: 10,
    cellWidth: "17",
    cellHeight: "21",
    showPHgoCoordinates: true,
    showLengthRatio: true,
    showLengthPercent: true,
    showAlignmentColumnNumbers: false,
    columnNumberInterval: "37",
    showRightResidueNumbers: true,
    showGroups: false,
    showFeatures: false
  });
  const layout = api.resolveLayout(settings, model);
  api.renderScene(model, settings, layout, {
    renderMSAExportScene(receivedSettings, receivedLayout) {
      assert.equal(receivedSettings.format, "pdf");
      assert.equal(receivedSettings.scale, 10);
      assert.equal(receivedSettings.cellWidth, 17);
      assert.equal(receivedSettings.cellHeight, 21);
      assert.equal(receivedSettings.showPHgoCoordinates, true);
      assert.equal(receivedSettings.showLengthRatio, true);
      assert.equal(receivedSettings.showLengthPercent, true);
      assert.equal(receivedSettings.showAlignmentColumnNumbers, false);
      assert.equal(receivedSettings.columnNumberInterval, 37);
      assert.equal(receivedSettings.showRightResidueNumbers, true);
      assert.equal(receivedSettings.showGroups, false);
      assert.equal(receivedSettings.showFeatures, false);
      assert.equal(receivedLayout, layout);
      return { svg: "<svg data-msaexpor=\"1\" data-msaexpor-renderer=\"jalview-native\"></svg>", width: 20, height: 10, source: "jalview-native" };
    }
  });
}

{
  const settings = api.normalizeSettings({
    showRightResidueNumbers: false,
    useAdvancedLayoutScript: true,
    advancedLayoutScript: ">1,4\\10,20"
  });
  assert.equal(settings.showRightResidueNumbers, true);
  const layout = api.resolveLayout(settings, model);
  assert.equal(layout.blocks.length, 1);
}

{
  const shell = api.appShell();
  assert.match(shell, /Refresh preview/);
  assert.match(shell, /data-role="dirty"/);
  assert.match(shell, /Generate/);
  assert.match(shell, /Cancel/);
}

{
  assert.equal(typeof api.validateModel, "function");
}

{
  assert.equal(api.titleNumberFilenameBase("Phgomsar: 1.1 Some MSA"), "1.1");
  assert.equal(api.titleNumberFilenameBase("Phgotreer: 1 Tree"), "1");
  assert.equal(api.titleNumberFilenameBase("Phgomsar: 1/2 invalid"), "1");
  assert.equal(api.defaultExportBaseName({ title: "2.3 Export" }, "", "session-name"), "2.3");
  assert.equal(api.defaultExportBaseName({}, "Phgomsar: no number", "session/id"), "session_id");
  assert.equal(api.defaultExportBaseName({}, "Phgomsar: no number", "///"), "msa");
}

{
  const calls = [];
  context.window.isSecureContext = true;
  context.window.showSaveFilePicker = async (options) => {
    calls.push({ picker: options });
    return {
      async createWritable() {
        calls.push({ createWritable: true });
        return {
          async write(blob) {
            calls.push({ writeType: blob.type, writeSize: blob.size });
          },
          async close() {
            calls.push({ close: true });
          }
        };
      }
    };
  };
  const settings = api.normalizeSettings({ format: "svg" });
  const target = await api.prepareSaveTargetForSettings(settings, "1.1");
  assert.equal(calls.length, 1);
  assert.equal(calls[0].picker.suggestedName, "1.1.svg");
  await api.exportScene({ svg: "<svg data-msaexpor=\"1\"></svg>", width: 1, height: 1 }, settings, "1.1", target);
  assert.equal(calls.filter((call) => call.picker).length, 1);
  assert.equal(calls.some((call) => call.writeType === "image/svg+xml;charset=utf-8" && call.writeSize > 0), true);
  delete context.window.showSaveFilePicker;
  delete context.window.isSecureContext;
}

{
  context.window.isSecureContext = true;
  context.window.showSaveFilePicker = async () => {
    const error = new Error("canceled");
    error.name = "AbortError";
    throw error;
  };
  const target = await api.prepareSaveTargetForSettings(api.normalizeSettings({ format: "svg" }), "1.1");
  assert.equal(target, false);
  delete context.window.showSaveFilePicker;
  delete context.window.isSecureContext;
}

{
  delete context.window.__PHGO_SAVE_BLOB__;
  delete context.window.showSaveFilePicker;
  delete context.window.isSecureContext;
  await assert.rejects(
    () => api.prepareSaveTargetForSettings(api.normalizeSettings({ format: "svg" }), "1.1"),
    /requires the PHgo save bridge or the browser save-file picker/
  );
  await assert.rejects(
    () => api.exportScene({ svg: "<svg data-msaexpor=\"1\"></svg>", width: 1, height: 1 }, api.normalizeSettings({ format: "svg" }), "1.1"),
    /requires a prepared save target/
  );
}

console.log("msaexpor tests passed");
