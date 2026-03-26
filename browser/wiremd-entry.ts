import { parse } from "wiremd/parser";
import { renderToHTML } from "wiremd/renderer";

interface WiremdResult {
  html: string;
  css: string;
}

function render(source: string, style: string = "sketch"): WiremdResult {
  const ast = parse(source);
  const doc = renderToHTML(ast, { style, inlineStyles: true, pretty: false });

  const bodyMatch = doc.match(/<body[^>]*>([\s\S]*)<\/body>/);
  const html = bodyMatch ? bodyMatch[1] : "";

  const styleMatch = doc.match(/<style>([\s\S]*?)<\/style>/);
  let css = styleMatch ? styleMatch[1] : "";
  css = css.replace(/body\.wmd-/g, ".wmd-");

  return { html, css };
}

(window as any).wiremd = { render };
