// Markdown renders user-written text, such as a Ticket description, as GFM.
// Raw HTML stays text, task-list checkboxes are display-only, and links open
// in a new tab, except an ID Link, a link whose target is a Spec's or
// Ticket's full ID, which opens it in place (see idLinks.tsx).
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { NO_ID_LINKS, useIdLinks } from "./idLinks";
import { hrefFor, idLinkPatch, isIdLink, opensInPlace } from "./url";

const components: Components = {
  a: function Anchor({ node: _node, href, ...props }) {
    const ids = useIdLinks();
    if (!href || !isIdLink(href)) return <a {...props} href={href} target="_blank" rel="noopener noreferrer" />;
    const { loc, navigate, onNavigate, newTab } = ids ?? NO_ID_LINKS;
    const patch = idLinkPatch(loc, href);
    const to = hrefFor({ ...loc, ...patch });
    if (newTab) return <a {...props} href={to} target="_blank" rel="noopener noreferrer" />;
    return (
      <a
        {...props}
        href={to}
        onClick={(e) => {
          if (!ids || !opensInPlace(e)) return;
          e.preventDefault();
          navigate(patch);
          onNavigate?.();
        }}
      />
    );
  },
};

export function Markdown({ children }: { children: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
