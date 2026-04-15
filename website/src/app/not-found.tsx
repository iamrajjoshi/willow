import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center">
      <h1 className="font-heading text-4xl font-bold text-willow-text-1">
        404
      </h1>
      <p className="mt-2 text-willow-text-3">Page not found.</p>
      <Link
        href="/"
        className="mt-6 text-sm text-willow-accent hover:underline"
      >
        Go home
      </Link>
    </main>
  );
}
