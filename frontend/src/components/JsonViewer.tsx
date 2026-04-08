import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronRight, Copy, CheckCheck } from "lucide-react";

interface JsonViewerProps {
  data: unknown;
  maxLines?: number;
  forceExpanded?: boolean;
}

function syntaxHighlight(json: string): string {
  return json.replace(
    /("(\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let cls = "text-orange-400";
      if (/^"/.test(match)) {
        cls = /:$/.test(match) ? "text-blue-400" : "text-emerald-400";
      } else if (/true|false/.test(match)) {
        cls = "text-violet-400";
      } else if (/null/.test(match)) {
        cls = "text-zinc-500";
      }
      return `<span class="${cls}">${match}</span>`;
    }
  );
}

export function JsonViewer({ data, maxLines = 20, forceExpanded }: JsonViewerProps) {
  const formatted = JSON.stringify(data, null, 2);
  const lines = formatted.split("\n");
  const isLong = lines.length > maxLines;
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (forceExpanded !== undefined) {
      setExpanded(forceExpanded);
    }
  }, [forceExpanded]);

  const displayText = expanded ? formatted : lines.slice(0, maxLines).join("\n") + "\n…";
  const highlighted = syntaxHighlight(displayText);

  return (
    <div className="relative">
      <button
        type="button"
        className="absolute top-2 right-2 z-10 p-1.5 rounded-md bg-zinc-800 hover:bg-zinc-700 transition-colors text-zinc-400 hover:text-zinc-200"
        onClick={() => {
          const textarea = document.createElement("textarea");
          textarea.value = formatted;
          textarea.style.position = "fixed";
          textarea.style.opacity = "0";
          document.body.appendChild(textarea);
          textarea.select();
          document.execCommand("copy");
          document.body.removeChild(textarea);
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        }}
      >
        {copied ? (
          <span className="flex items-center gap-1 text-emerald-400 text-xs">
            <CheckCheck className="h-3.5 w-3.5" />
            Copied!
          </span>
        ) : (
          <Copy className="h-3.5 w-3.5" />
        )}
      </button>
      <pre className="rounded-lg bg-zinc-950 p-4 text-xs leading-relaxed font-mono text-zinc-300 whitespace-pre-wrap break-all">
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
      {isLong && (
        <Button
          variant="ghost"
          size="sm"
          className="mt-1 text-xs text-muted-foreground"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? (
            <>
              <ChevronDown className="mr-1 h-3 w-3" /> Collapse
            </>
          ) : (
            <>
              <ChevronRight className="mr-1 h-3 w-3" /> Show all {lines.length} lines
            </>
          )}
        </Button>
      )}
    </div>
  );
}
