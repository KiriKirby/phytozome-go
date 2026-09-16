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
