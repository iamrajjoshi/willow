import { NavBar } from "@/components/NavBar";
import { Footer } from "@/components/Footer";
import { Sidebar } from "@/components/docs/Sidebar";
import { MobileSidebar } from "@/components/docs/MobileSidebar";
import { PrevNextLinks } from "@/components/docs/PrevNextLinks";

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <NavBar />
      <div className="mx-auto flex max-w-6xl px-6 pt-24">
        {/* Desktop sidebar with page nav + scroll-spy ToC */}
        <aside className="hidden w-56 shrink-0 lg:block">
          <div className="sticky top-24 max-h-[calc(100vh-8rem)] overflow-y-auto">
            <Sidebar />
          </div>
        </aside>

        {/* Main content */}
        <main className="min-w-0 flex-1 pb-16 lg:pl-10">
          {/* Mobile sidebar toggle */}
          <div className="mb-6 lg:hidden">
            <MobileSidebar />
          </div>

          <article className="prose prose-invert max-w-none pb-[50vh]">
            {children}
          </article>
          <PrevNextLinks />
        </main>
      </div>
      <Footer />
    </>
  );
}
