import { useState, useMemo, useRef, useEffect } from "react";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ChevronRight, ChevronDown, Plus, X, Search } from "lucide-react";
import { cn } from "@/lib/utils";

interface FieldNode {
  name: string;
  path: string;
  isLeaf: boolean;
  isArray: boolean;
  children: FieldNode[];
}

interface ActiveFilter {
  field: string;
  value: string;
}

interface FieldFilterPopoverProps {
  fields: string[];
  activeFilters: ActiveFilter[];
  onAddFilter: (field: string, value: string) => void;
  onRemoveFilter: (field: string) => void;
}

function buildFieldTree(fields: string[]): FieldNode[] {
  const root: FieldNode = { name: "", path: "", isLeaf: false, isArray: false, children: [] };

  for (const field of fields) {
    const segments = field.split(".");
    let node = root;
    let accumulated = "";

    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i];
      const isArray = seg.endsWith("[]");
      const name = isArray ? seg.slice(0, -2) : seg;
      accumulated = accumulated ? `${accumulated}.${seg}` : seg;

      let child = node.children.find((c) => c.name === name && c.isArray === isArray);
      if (!child) {
        child = {
          name,
          path: accumulated,
          isLeaf: i === segments.length - 1,
          isArray,
          children: [],
        };
        node.children.push(child);
      }
      if (i === segments.length - 1) {
        child.isLeaf = true;
      }
      node = child;
    }
  }

  sortTree(root);
  return root.children;
}

function sortTree(node: FieldNode): void {
  node.children.sort((a, b) => {
    if (a.isLeaf !== b.isLeaf) return a.isLeaf ? 1 : -1;
    return a.name.localeCompare(b.name);
  });
  for (const child of node.children) {
    sortTree(child);
  }
}

function filterTree(nodes: FieldNode[], query: string): FieldNode[] {
  if (!query) return nodes;
  const lower = query.toLowerCase();
  const result: FieldNode[] = [];
  for (const node of nodes) {
    const matches = node.path.toLowerCase().includes(lower);
    const filteredChildren = filterTree(node.children, query);
    if (matches || filteredChildren.length > 0) {
      result.push({ ...node, children: filteredChildren.length > 0 ? filteredChildren : node.children });
    }
  }
  return result;
}

function collectExpandedPaths(nodes: FieldNode[], acc: Set<string>): void {
  for (const node of nodes) {
    if (node.children.length > 0) {
      acc.add(node.path);
      collectExpandedPaths(node.children, acc);
    }
  }
}

function lastSegment(path: string): string {
  const segments = path.split(".");
  const last = segments[segments.length - 1];
  return last;
}

function TreeView({
  nodes,
  depth,
  expanded,
  toggleExpanded,
  selectedField,
  onSelectField,
}: {
  nodes: FieldNode[];
  depth: number;
  expanded: Set<string>;
  toggleExpanded: (path: string) => void;
  selectedField: string | null;
  onSelectField: (path: string) => void;
}) {
  return (
    <>
      {nodes.map((node) => {
        const hasChildren = node.children.length > 0;
        const isExpanded = expanded.has(node.path);
        const isSelected = selectedField === node.path;
        const displayName = node.isArray ? `${node.name}[]` : node.name;

        if (hasChildren) {
          return (
            <div key={node.path}>
              <button
                className={cn(
                  "w-full flex items-center gap-1 px-2 py-1 text-left text-xs hover:bg-accent rounded-sm",
                  isSelected && "bg-accent",
                )}
                style={{ paddingLeft: `${depth * 12 + 8}px` }}
                onClick={() => toggleExpanded(node.path)}
              >
                {isExpanded ? (
                  <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                )}
                <span
                  className="font-mono truncate flex-1"
                  title={node.path}
                >
                  {displayName}
                </span>
                {node.isArray && (
                  <span className="text-muted-foreground text-[10px] shrink-0">array</span>
                )}
              </button>
              {isExpanded && (
                <TreeView
                  nodes={node.children}
                  depth={depth + 1}
                  expanded={expanded}
                  toggleExpanded={toggleExpanded}
                  selectedField={selectedField}
                  onSelectField={onSelectField}
                />
              )}
            </div>
          );
        }

        return (
          <button
            key={node.path}
            className={cn(
              "w-full flex items-center gap-1 px-2 py-1 text-left text-xs hover:bg-accent rounded-sm",
              isSelected ? "bg-accent ring-1 ring-ring" : "",
            )}
            style={{ paddingLeft: `${depth * 12 + 20}px` }}
            onClick={() => onSelectField(node.path)}
          >
            <span
              className="font-mono truncate flex-1"
              title={node.path}
            >
              {displayName}
            </span>
          </button>
        );
      })}
    </>
  );
}

export function FieldFilterPopover({
  fields,
  activeFilters,
  onAddFilter,
  onRemoveFilter,
}: FieldFilterPopoverProps) {
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedField, setSelectedField] = useState<string | null>(null);
  const [value, setValue] = useState("");
  const [treeQuery, setTreeQuery] = useState("");
  const valueRef = useRef<HTMLInputElement>(null);
  const treeSearchRef = useRef<HTMLInputElement>(null);

  const fullTree = useMemo(() => buildFieldTree(fields), [fields]);

  const filteredTree = useMemo(() => {
    if (!treeQuery.trim()) return fullTree;
    return filterTree(fullTree, treeQuery.trim());
  }, [fullTree, treeQuery]);

  useEffect(() => {
    if (treeQuery.trim()) {
      const allExpanded = new Set<string>();
      collectExpandedPaths(filteredTree, allExpanded);
      setExpanded(allExpanded);
    }
  }, [treeQuery, filteredTree]);

  useEffect(() => {
    if (selectedField && open) {
      const segments = selectedField.split(".");
      const pathSet = new Set<string>();
      let accumulated = "";
      for (let i = 0; i < segments.length - 1; i++) {
        accumulated = accumulated ? `${accumulated}.${segments[i]}` : segments[i];
        pathSet.add(accumulated);
      }
      setExpanded((prev) => new Set([...prev, ...pathSet]));
    }
  }, [selectedField, open]);

  useEffect(() => {
    if (open && selectedField) {
      setTimeout(() => valueRef.current?.focus(), 50);
    }
    if (open && !selectedField) {
      setTimeout(() => treeSearchRef.current?.focus(), 50);
    }
  }, [open, selectedField]);

  const toggleExpanded = (path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  const handleAdd = () => {
    if (!selectedField || !value.trim()) return;
    onAddFilter(selectedField, value.trim());
    setValue("");
    setSelectedField(null);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <Plus className="h-3.5 w-3.5" />
          {activeFilters.length > 0 ? `Filters (${activeFilters.length})` : "Add Filter"}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 p-0" align="start">
        <div className="flex flex-col max-h-96">
          <div className="p-2 border-b">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                ref={treeSearchRef}
                placeholder="Search fields…"
                value={treeQuery}
                onChange={(e) => setTreeQuery(e.target.value)}
                className="h-8 text-xs pl-8"
              />
              {treeQuery && (
                <button
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  onClick={() => setTreeQuery("")}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          </div>

          <div className="flex-1 overflow-auto py-1 min-h-[120px] max-h-64">
            {filteredTree.length === 0 ? (
              <p className="px-3 py-4 text-xs text-muted-foreground text-center">
                {treeQuery ? "No matching fields" : "No fields available"}
              </p>
            ) : (
              <TreeView
                nodes={filteredTree}
                depth={0}
                expanded={expanded}
                toggleExpanded={toggleExpanded}
                selectedField={selectedField}
                onSelectField={(path) => {
                  setSelectedField(path);
                  setValue("");
                }}
              />
            )}
          </div>

          {activeFilters.length > 0 && (
            <div className="border-t px-2 py-1.5 space-y-1 max-h-20 overflow-auto">
              {activeFilters.map((f) => (
                <div
                  key={f.field}
                  className="flex items-center gap-1 text-xs bg-secondary rounded px-1.5 py-0.5"
                >
                  <span
                    className="font-mono text-muted-foreground truncate flex-1"
                    title={f.field}
                  >
                    {f.field}
                  </span>
                  <span className="font-mono truncate max-w-24" title={f.value}>
                    {f.value}
                  </span>
                  <button
                    className="shrink-0 rounded-full hover:bg-muted p-0.5"
                    onClick={() => onRemoveFilter(f.field)}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ))}
            </div>
          )}

          <div className="border-t p-2 flex gap-1.5">
            <Input
              ref={valueRef}
              placeholder={
                selectedField ? `${lastSegment(selectedField)}:` : "Select a field…"
              }
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  handleAdd();
                }
              }}
              disabled={!selectedField}
              className="h-8 text-xs flex-1"
            />
            <Button
              size="sm"
              className="h-8 px-2"
              onClick={handleAdd}
              disabled={!selectedField || !value.trim()}
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
