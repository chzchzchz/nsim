// sanitize.ts — HTML sanitization for remote-server content.
//
// Remote data (controls, sensors, logs, alarms) arrives as JSON
// from an nsimserve process that may be operated by someone else.
// Inserting it with innerHTML would let a hostile server inject
// arbitrary markup (<img onerror>, <script>, ...). The UI must
// therefore treat every remote string as text.
//
// These helpers are intentionally dependency-free:
// - escapeHtml() for plain text (logs, part labels rendered via
//   innerHTML for <wbr> wrapping).
// - sanitizeMarkdownHtml() for descriptions rendered through
//   marked.parse(): parse → strip scripts/styles/iframes/objects
//   and all on* attributes + javascript: URIs → serialize. This
//   keeps emphasis/links from the bundled description markdown
//   while neutralizing embedded HTML.

const DANGEROUS_TAGS = new Set([
    "script", "style", "iframe", "object", "embed", "form",
    "link", "meta", "base", "template", "noscript",
    // SVG/MathML vectors: mXSS-safe by removing the carriers.
    "foreignobject", "annotation-xml", "mi", "mo", "mn", "ms", "mtext",
    "title",
    // SVG/MathML integration points: when sanitizer output becomes
    // a child's innerHTML, native html-derived tags keep working,
    // but SVG descendants must still be walked. Remove the SVG
    // carriers and their descendants wholesale.
    "svg", "math", "mrow", "mstyle", "merror", "mfrac", "msqrt",
    "mroot", "mtable", "maction",
    // SVG subset: animate/callers can execute javascript: URIs.
    "animate", "set", "animatetransform", "animatemotion",
    "image", "audio", "video", "source", "track",
    "applet", "marquee", "frame", "frameset",
    // Inline-handler carriers inside SVG: <use> may reach an
    // xlink:href that lands on javascript:.
    "use",
]);

function normalizeScheme(v: string): string {
    // Browsers ignore ASCII control characters (0x00-0x1F) before
    // resolving schemes, so "java\tscript:" still executes.
    // Strip them before the prefix check.
    return v.replace(/[\x00-\x1F\x7F]/g, "");
}



export function escapeHtml(text: string): string {
    return text
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
}

export function sanitizeMarkdownHtml(html: string): string {
    // Parse in an inert template so no image/script resources
    // load during sanitization.
    const tpl = document.createElement("template");
    tpl.innerHTML = html;
    const frag = tpl.content;

    const kill = (el: Element): void => {
        el.remove();
    };
    // Remove dangerous subtrees first (walking a snapshot since
    // removal mutates the tree).
    const all = Array.from(frag.querySelectorAll("*"));
    for (const el of all) {
        if (DANGEROUS_TAGS.has(el.tagName.toLowerCase())) {
            kill(el);
            continue;
        }
        // Style attributes can carry url() javascript: vectors;
        // in SVGs the same attribute uses the namespaced
        // xlink:href form. Only URL-carrying attributes are
        // scheme-checked; a colon in the name (xlink:, on*, etc.)
        // is always removed as well. Control-character scrubbing
        // is scoped to URL attrs so benign text in title/alt/aria-*
        // is not mangled.
        for (const attr of Array.from(el.attributes)) {
            const name = attr.name.toLowerCase();
            if (name.startsWith("on")) {
                el.removeAttribute(attr.name);
                continue;
            }
            const isUrlAttr =
                name === "href" || name === "src" ||
                name === "xlink:href" || name === "formaction" ||
                name === "action" || name === "style";
            if (isUrlAttr) {
                const v = normalizeScheme(attr.value).toLowerCase();
                if (
                    v.startsWith("javascript:") ||
                    v.startsWith("data:") ||
                    v.startsWith("vbscript:") ||
                    v.includes("url(")
                ) {
                    el.removeAttribute(attr.name);
                    continue;
                }
            }
            if (name.includes(":")) {
                el.removeAttribute(attr.name);
            }
        }
    }
    // Serialize back via a div wrapper.
    const div = document.createElement("div");
    div.appendChild(frag.cloneNode(true));
    return div.innerHTML;
}
