export function Footer() {
  return (
    <footer className="border-t border-willow-border">
      <div className="mx-auto flex max-w-6xl items-center justify-center px-6 py-8">
        <div className="flex items-center gap-4 text-sm text-willow-text-3">
          <a
            href="https://github.com/iamrajjoshi/willow/blob/main/LICENSE"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-colors hover:text-willow-text-2"
          >
            MIT
          </a>
          <span className="text-willow-text-dim">·</span>
          <a
            href="https://github.com/iamrajjoshi"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-colors hover:text-willow-text-2"
          >
            @iamrajjoshi
          </a>
        </div>
      </div>
    </footer>
  );
}
