export default {
  join: (...args: string[]) => args.join("/"),
  basename: (p: string) => p.split("/").pop() || "",
  dirname: (p: string) => p.split("/").slice(0, -1).join("/") || "/",
  extname: (p: string) => { const m = p.match(/\.[^.]+$/); return m ? m[0] : ""; },
  resolve: (...args: string[]) => args.join("/"),
  sep: "/",
};
