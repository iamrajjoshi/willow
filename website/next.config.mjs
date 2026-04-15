import createMDX from "@next/mdx";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import rehypePrettyCode from "rehype-pretty-code";

const withMDX = createMDX({
  options: {
    remarkPlugins: [remarkGfm],
    rehypePlugins: [
      rehypeSlug,
      [rehypePrettyCode, { theme: "github-dark-default", keepBackground: false }],
    ],
  },
});

export default withMDX({
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
  pageExtensions: ["tsx", "ts", "mdx"],
});
