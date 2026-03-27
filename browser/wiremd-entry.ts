import { parse } from "wiremd/parser";
import { renderToHTML } from "wiremd/renderer";

interface WiremdResult {
  html: string;
  css: string;
}

function scopeCSS(css: string): string {
  const imports: string[] = [];
  const rest = css.replace(/@import\s+url\([^)]+\)\s*;/g, (m: string) => {
    imports.push(m);
    return "";
  });
  return imports.join("\n") + "\n.wiremd-rendered {\n" + rest + "\n}\n";
}

export function render(source: string, style: string = "sketch"): WiremdResult {
  const ast = parse(source);
  const doc = renderToHTML(ast, { style, inlineStyles: true, pretty: false });

  const bodyMatch = doc.match(/<body[^>]*>([\s\S]*)<\/body>/);
  const html = bodyMatch ? bodyMatch[1] : "";

  const styleMatch = doc.match(/<style>([\s\S]*?)<\/style>/);
  let css = styleMatch ? styleMatch[1] : "";
  css = css.replace(/body\.wmd-/g, ".wmd-");
  css = scopeCSS(css);

  return { html, css };
}

