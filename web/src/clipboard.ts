// clipboard.ts — copy text to the clipboard with the async API
// and fall back to a hidden textarea + execCommand for hosts
// where the clipboard permission is unavailable.

export async function copyText(text: string): Promise<boolean> {
    try {
        await navigator.clipboard.writeText(text);
        return true;
    } catch {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.left = "-9999px";
        ta.style.opacity = "0";
        ta.readOnly = false;
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        const ok = document.execCommand("copy");
        document.body.removeChild(ta);
        if (!ok) {
            console.error("copy returned false");
        }
        return ok;
    }
}
