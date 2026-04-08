import { useState } from "react";
import { createPatch } from "diff";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Diff, ChevronDown, ChevronRight } from "lucide-react";
import type { Message } from "@/lib/api";

interface MessageDiffProps {
  prev: Message;
  current: Message;
}

export function MessageDiff({ prev, current }: MessageDiffProps) {
  const [open, setOpen] = useState(true);

  const prevJson = JSON.stringify(prev.body, null, 2);
  const currJson = JSON.stringify(current.body, null, 2);

  if (prevJson === currJson) {
    return (
      <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
        <Diff className="h-3 w-3" />
        No changes
      </div>
    );
  }

  const patch = createPatch(
    "body",
    prevJson,
    currJson,
    `offset ${prev.offset}`,
    `offset ${current.offset}`
  );

  const diffLines = patch.split("\n").slice(4);
  const additions = diffLines.filter(l => l.startsWith("+")).length;
  const deletions = diffLines.filter(l => l.startsWith("-")).length;

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
        >
          <Diff className="h-3 w-3" />
          {open ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
          {open ? "Hide diff" : "Show diff"}
          <span className="text-muted-foreground/70 ml-1">
            ({additions > 0 && <span className="text-emerald-400">+{additions}</span>}
            {additions > 0 && deletions > 0 && " "}
            {deletions > 0 && <span className="text-red-400">-{deletions}</span>})
          </span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-1 rounded-lg bg-zinc-950 p-3 text-xs leading-relaxed font-mono whitespace-pre-wrap break-all">
          {diffLines.map((line, i) => {
            let className = "text-zinc-300";
            if (line.startsWith("+")) className = "text-emerald-400";
            else if (line.startsWith("-")) className = "text-red-400";
            else if (line.startsWith("@@")) className = "text-blue-400";
            return (
              <div key={i} className={className}>
                {line}
              </div>
            );
          })}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}
