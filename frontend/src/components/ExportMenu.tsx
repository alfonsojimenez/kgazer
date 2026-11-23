import { useState } from "react";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { ExportFormat } from "@/lib/api";

interface ExportMenuProps {
  label?: string;
  description?: string;
  urlFor: (format: ExportFormat) => string;
}

export function ExportMenu({ label = "Export", description, urlFor }: ExportMenuProps) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm">
          <Download className="h-3.5 w-3.5" />
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-2" align="end">
        {description && <p className="px-2 pb-2 text-xs text-muted-foreground">{description}</p>}
        {(["json", "csv"] as const).map((format) => (
          <a
            key={format}
            href={urlFor(format)}
            download
            onClick={() => setOpen(false)}
            className="block rounded-sm px-2 py-1.5 text-sm hover:bg-accent"
          >
            Download {format.toUpperCase()}
          </a>
        ))}
      </PopoverContent>
    </Popover>
  );
}
