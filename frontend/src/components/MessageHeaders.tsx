import { useState } from "react";
import { ChevronDown, ChevronRight, Tag } from "lucide-react";
import type { MessageHeader } from "@/lib/api";

export function MessageHeaders({ headers }: { headers?: MessageHeader[] }) {
  const [open, setOpen] = useState(false);
  if (!headers || headers.length === 0) return null;

  return (
    <div className="mb-3">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
      >
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        <Tag className="h-3 w-3" />
        {headers.length} header{headers.length !== 1 ? "s" : ""}
      </button>
      {open && (
        <table className="mt-2 w-full text-xs font-mono">
          <tbody>
            {headers.map((h, i) => (
              <tr key={i} className="border-b last:border-0 align-top">
                <td className="py-1 pr-4 text-muted-foreground whitespace-nowrap">{h.key}</td>
                <td className="py-1 break-all">
                  {h.value === null ? (
                    <span className="italic text-muted-foreground">null</span>
                  ) : (
                    <>
                      {h.value}
                      {h.base64 && <span className="ml-2 text-muted-foreground not-italic">(base64)</span>}
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
