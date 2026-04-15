import type { Metadata } from "next";
import { Inter, Poppins, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const poppins = Poppins({
  subsets: ["latin"],
  weight: ["600", "700", "800"],
  variable: "--font-heading",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL("https://getwillow.dev"),
  title: { default: "willow", template: "%s | willow" },
  description: "A git worktree manager built for AI agent workflows.",
  openGraph: {
    title: "willow — Git worktree manager for AI agents",
    description:
      "Spin up isolated worktrees for Claude Code sessions. Switch between them instantly with fzf. See which agents are busy, waiting, or idle.",
    type: "website",
    url: "https://getwillow.dev",
  },
  icons: { icon: "/favicon.svg" },
  other: { "theme-color": "#0a0a0f" },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${poppins.variable} ${jetbrainsMono.variable}`}
    >
      <body className="font-sans">{children}</body>
    </html>
  );
}
