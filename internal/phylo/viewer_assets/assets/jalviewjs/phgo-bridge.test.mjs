import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const source = fs.readFileSync(new URL("./phgo-bridge.js", import.meta.url), "utf8");
const context = {
  window: {},
  document: {},
  console,
  URL,
  setTimeout,
  clearTimeout,
  setInterval: () => 0,
  clearInterval: () => {}
};
context.window.setTimeout = setTimeout;
context.window.clearTimeout = clearTimeout;
context.window.setInterval = context.setInterval;
context.window.clearInterval = context.clearInterval;
vm.createContext(context);
vm.runInContext(source, context, { filename: "phgo-bridge.js" });

const styleFor = (viewportOptions, group = null) => {
  const colour = {
    getRed$: () => 255,
    getGreen$: () => 0,
    getBlue$: () => 0,
    darker$: () => ({ getRed$: () => 178, getGreen$: () => 0, getBlue$: () => 0 })
  };
  const shader = {
    getColourScheme$: () => ({}),
    findColour$C$I$jalview_datamodel_SequenceI: () => colour
  };
  const viewport = {
    getShowBoxes$: () => viewportOptions.boxes,
    getShowText$: () => viewportOptions.text,
    getColourText$: () => viewportOptions.colourText,
    getTextColour$: () => ({ getRed$: () => 17, getGreen$: () => 17, getBlue$: () => 17 }),
    getResidueShading$: () => shader
  };
  const alignment = {
    findAllGroups$jalview_datamodel_SequenceI: () => group ? [group] : []
  };
  const sequence = {
    getLength$: () => 1,
    getCharAt$I: () => "A"
  };
  return context.window.PHGOJalviewBridge.testHooks.residueFormatStyle(viewport, alignment, sequence, 0);
};

{
  const style = styleFor({ boxes: false, text: true, colourText: true });
  assert.equal(style.showBoxes, false);
  assert.equal(style.showText, true);
  assert.equal(style.textFill, "#ff0000");
}

{
  const style = styleFor({ boxes: true, text: true, colourText: true });
  assert.equal(style.showBoxes, true);
  assert.equal(style.showText, true);
  assert.equal(style.textFill, "#b20000");
}

{
  const style = styleFor({ boxes: false, text: false, colourText: true });
  assert.equal(style.showBoxes, false);
  assert.equal(style.showText, false);
}

{
  const group = {
    getStartRes$: () => 0,
    getEndRes$: () => 0,
    getDisplayBoxes$: () => true,
    getDisplayText$: () => false,
    getColourText$: () => false,
    getTextColour$: () => ({ getRed$: () => 0, getGreen$: () => 0, getBlue$: () => 255 }),
    getGroupColourScheme$: () => ({ getColourScheme$: () => ({}) })
  };
  const style = styleFor({ boxes: false, text: true, colourText: true }, group);
  assert.equal(style.showBoxes, true);
  assert.equal(style.showText, false);
  assert.equal(style.textFill, "#0000ff");
}

console.log("phgo bridge format tests passed");

{
  const hook = context.window.PHGOJalviewBridge.testHooks.featureTextColour;
  assert.equal(hook(["#ff0000"], "#111111"), "#ffffff");
  assert.equal(hook(["#ffff00"], "#111111"), "#111111");
  assert.equal(hook([], "#123456"), "#123456");
}

{
  const layout = context.window.PHGOJalviewBridge.testHooks.layoutFeatureLegend(
    [
      { name: "Active site", colour: "#ff0000" },
      { name: "Transmembrane helix", colour: "#0080ff" },
      { name: "Binding region", colour: "#00aa44" }
    ],
    50,
    (text) => text.length * 7,
    12
  );
  assert.equal(layout.items.length, 3);
  assert.equal(layout.width, 172);
  assert.equal(layout.height, 77);
  assert.deepEqual(Array.from(layout.items, (item) => item.x), [layout.padding, layout.padding, layout.padding]);
  assert.deepEqual(Array.from(layout.items, (item) => item.y), [0, 19, 38]);
}

{
  const colour = (red, green, blue) => ({
    getRed$: () => red,
    getGreen$: () => green,
    getBlue$: () => blue
  });
  const feature = (type, begin, end, paint) => ({
    getType$: () => type,
    getBegin$: () => begin,
    getEnd$: () => end,
    paint
  });
  const activeSite = feature("Active site", 2, 3, colour(255, 0, 0));
  const duplicateActiveSite = feature("Active site", 6, 7, colour(255, 0, 0));
  const bindingRegion = feature("Binding region", 4, 5, colour(0, 170, 68));
  const outsideRange = feature("Outside", 20, 25, colour(0, 0, 255));
  const sequence = {
    getSequenceFeatures$: () => [activeSite, duplicateActiveSite, bindingRegion, outsideRange],
    findIndex$I: (position) => position
  };
  const renderer = {
    getRenderOrder$: () => ["Binding region", "Active site", "Outside"],
    showFeatureOfType$S: () => true,
    featureGroupNotShown$jalview_datamodel_SequenceFeature: () => false,
    isVisible$jalview_datamodel_SequenceFeature: () => true,
    getFeatureStyle$S: () => ({}),
    getColor$jalview_datamodel_SequenceFeature$jalview_api_FeatureColourI: (item) => item.paint
  };
  const entries = context.window.PHGOJalviewBridge.testHooks.collectFeatureLegendEntries(
    { getFeatureRenderer$: () => renderer },
    null,
    { getSequenceAt$I: () => sequence },
    [{ block: { columnStartBoundary: 0, columnEndBoundary: 8, visibleColumnCount: 8 }, indexes: [0] }],
    { showFeatures: true }
  );
  assert.deepEqual(
    Array.from(entries, (entry) => [entry.name, entry.colour]),
    [["Binding region", "#00aa44"], ["Active site", "#ff0000"]]
  );
  assert.equal(
    context.window.PHGOJalviewBridge.testHooks.collectFeatureLegendEntries(
      { getFeatureRenderer$: () => renderer },
      null,
      { getSequenceAt$I: () => sequence },
      [{ block: { columnStartBoundary: 0, columnEndBoundary: 8, visibleColumnCount: 8 }, indexes: [0] }],
      { showFeatures: false }
    ).length,
    0
  );
}

{
  const colours = context.window.PHGOJalviewBridge.testHooks.featureCellColours(
    null,
    null,
    { getCharAt$I: () => "-", getLength$: () => 1 },
    0,
    { showFeatures: true }
  );
  assert.deepEqual(Array.from(colours), []);
}
