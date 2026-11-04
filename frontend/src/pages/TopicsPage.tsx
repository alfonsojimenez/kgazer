import { useState, useMemo, useRef, useEffect, useLayoutEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import { fetchTopics, fetchClusters, fetchSettingsInfo } from "@/lib/api";
import type { ClusterInfo, TopicSummary } from "@/lib/api";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusDot } from "@/components/StatusDot";
import { ArrowDown, ArrowUp, Check, Database, Search, X } from "lucide-react";

type SortColumn = "name" | "messages" | "keys" | "last_updated";
type SortDir = "asc" | "desc";

const SORT_COLUMNS: SortColumn[] = ["name", "messages", "keys", "last_updated"];
const DEFAULT_SORT: SortColumn = "last_updated";
const SYNCING_REFETCH_MS = 5_000;
const IDLE_REFETCH_MS = 30_000;
const VIRTUALIZE_THRESHOLD = 200;
const ROW_HEIGHT = 45;

function TableSkeleton() {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: 6 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}

function timeAgo(date: Date, now: number): string {
  const seconds = Math.floor((now - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

function formatLabel(format: string): string {
  const labels: Record<string, string> = { json: "JSON", avro: "Avro", protobuf: "Protobuf" };
  return labels[format?.toLowerCase()] ?? "Other";
}

function formatBadgeClass(format: string): string {
  const classes: Record<string, string> = {
    json: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20 text-[10px] px-1.5 py-0",
    avro: "bg-blue-500/10 text-blue-500 border-blue-500/20 text-[10px] px-1.5 py-0",
    protobuf: "bg-amber-500/10 text-amber-500 border-amber-500/20 text-[10px] px-1.5 py-0",
  };
  return classes[format?.toLowerCase()] ?? "text-[10px] px-1.5 py-0";
}

function isSyncing(topic: TopicSummary): boolean {
  return topic.progress.total > 0 && !topic.progress.done;
}

function compareTopics(a: TopicSummary, b: TopicSummary, column: SortColumn): number {
  switch (column) {
    case "name":
      return a.name.localeCompare(b.name);
    case "messages":
      return a.message_count - b.message_count;
    case "keys":
      return a.key_count - b.key_count;
    case "last_updated":
      return new Date(a.last_updated).getTime() - new Date(b.last_updated).getTime();
  }
}

function topicHref(topic: TopicSummary): string {
  return `/clusters/${encodeURIComponent(topic.cluster)}/topics/${encodeURIComponent(topic.name)}/keys`;
}

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

function SortableHead({
  column,
  label,
  className,
  sortColumn,
  sortDir,
  onSort,
}: {
  column: SortColumn;
  label: string;
  className?: string;
  sortColumn: SortColumn;
  sortDir: SortDir;
  onSort: (column: SortColumn) => void;
}) {
  const active = sortColumn === column;
  return (
    <TableHead
      className={`cursor-pointer select-none hover:text-foreground ${active ? "font-semibold text-foreground" : "text-muted-foreground"} ${className ?? ""}`}
      aria-sort={active ? (sortDir === "asc" ? "ascending" : "descending") : "none"}
      onClick={() => onSort(column)}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active && (sortDir === "desc" ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />)}
      </span>
    </TableHead>
  );
}

function EmptyState({
  cluster,
  clusterInfo,
  compactedOnly,
}: {
  cluster: string | undefined;
  clusterInfo: ClusterInfo | undefined;
  compactedOnly: boolean | undefined;
}) {
  if (!cluster) {
    return (
      <>
        <p className="font-medium text-foreground">No clusters configured</p>
        <p>Add a cluster under <code className="font-mono">kafka.clusters</code> in <code className="font-mono">config.yml</code> and restart KGazer.</p>
      </>
    );
  }
  if (clusterInfo?.status === "disconnected") {
    return (
      <>
        <p className="font-medium text-foreground">Can't reach {cluster}</p>
        <p>Check the bootstrap servers and credentials for this cluster in <code className="font-mono">config.yml</code>.</p>
      </>
    );
  }
  if (clusterInfo?.status === "connecting") {
    return <p className="font-medium text-foreground">Connecting to {cluster}…</p>;
  }
  if (clusterInfo?.paused) {
    return (
      <>
        <p className="font-medium text-foreground">Consumption is paused for {cluster}</p>
        <p>Resume the cluster to discover and consume topics.</p>
      </>
    );
  }
  return (
    <>
      <p className="font-medium text-foreground">No topics found</p>
      <p>
        Topics are discovered every 60 seconds.
        {compactedOnly && (
          <> Only compacted topics are shown because <code className="font-mono">compactedOnly</code> is enabled.</>
        )}
      </p>
    </>
  );
}

export function TopicsPage() {
  const navigate = useNavigate();
  const { cluster } = useParams<{ cluster: string }>();
  const [searchParams, setSearchParams] = useSearchParams();

  // Local state drives the input; the URL mirrors it. Reading the input value straight
  // from the URL drops keystrokes because router navigations are applied asynchronously.
  const [search, setSearchState] = useState(() => searchParams.get("q") ?? "");
  const sortParam = searchParams.get("sort") as SortColumn | null;
  const sortColumn: SortColumn = sortParam && SORT_COLUMNS.includes(sortParam) ? sortParam : DEFAULT_SORT;
  const defaultDir: SortDir = sortColumn === "name" ? "asc" : "desc";
  const dirParam = searchParams.get("dir");
  const sortDir: SortDir = dirParam === "asc" || dirParam === "desc" ? dirParam : defaultDir;

  function updateParams(changes: Record<string, string | null>) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [key, value] of Object.entries(changes)) {
          if (value) next.set(key, value);
          else next.delete(key);
        }
        return next;
      },
      { replace: true }
    );
  }

  function setSearch(value: string) {
    setSearchState(value);
    updateParams({ q: value || null });
  }

  const searchRef = useRef<HTMLInputElement>(null);
  const now = useNow(30_000);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "/" && !e.metaKey && !e.ctrlKey && !isTypingTarget(e.target)) {
        e.preventDefault();
        searchRef.current?.focus();
      }
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, []);

  const { data: topics, isLoading, error } = useQuery({
    queryKey: ["topics", cluster],
    queryFn: () => fetchTopics(cluster),
    refetchInterval: (query) =>
      query.state.data?.some(isSyncing) ? SYNCING_REFETCH_MS : IDLE_REFETCH_MS,
  });

  const { data: clusters } = useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
  });

  const clusterInfo = clusters?.find((c: ClusterInfo) => c.name === cluster);
  const hasNoTopics = !isLoading && !error && topics?.length === 0;

  const { data: settings } = useQuery({
    queryKey: ["settings-info"],
    queryFn: fetchSettingsInfo,
    enabled: hasNoTopics,
  });

  const filteredTopics = useMemo(() => {
    if (!topics) return [];
    const lower = search.toLowerCase();
    const filtered = search ? topics.filter((t) => t.name.toLowerCase().includes(lower)) : topics;
    return [...filtered].sort((a, b) => {
      const cmp = compareTopics(a, b, sortColumn);
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [topics, search, sortColumn, sortDir]);

  const totals = useMemo(
    () =>
      filteredTopics.reduce(
        (acc, t) => ({
          messages: acc.messages + t.message_count,
          keys: acc.keys + t.key_count,
          syncing: acc.syncing + (isSyncing(t) ? 1 : 0),
        }),
        { messages: 0, keys: 0, syncing: 0 }
      ),
    [filteredTopics]
  );

  const virtualize = filteredTopics.length > VIRTUALIZE_THRESHOLD;
  const tableRef = useRef<HTMLTableElement>(null);
  const [scrollMargin, setScrollMargin] = useState(0);
  useLayoutEffect(() => {
    if (!tableRef.current) return;
    setScrollMargin(tableRef.current.getBoundingClientRect().top + window.scrollY);
  }, [virtualize, isLoading]);

  const virtualizer = useWindowVirtualizer({
    count: virtualize ? filteredTopics.length : 0,
    enabled: virtualize,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
    scrollMargin,
  });

  const virtualItems = virtualizer.getVirtualItems();
  const visibleRows = virtualize
    ? virtualItems.map((item) => filteredTopics[item.index])
    : filteredTopics;
  const paddingTop = virtualize && virtualItems.length > 0 ? virtualItems[0].start - scrollMargin : 0;
  const paddingBottom =
    virtualize && virtualItems.length > 0
      ? virtualizer.getTotalSize() - (virtualItems[virtualItems.length - 1].end - scrollMargin)
      : 0;

  function handleSort(column: SortColumn) {
    if (sortColumn === column) {
      updateParams({ sort: column, dir: sortDir === "asc" ? "desc" : "asc" });
    } else {
      updateParams({ sort: column, dir: column === "name" ? "asc" : "desc" });
    }
  }

  function handleRowClick(e: React.MouseEvent, topic: TopicSummary) {
    if ((e.target as HTMLElement).closest("a")) return;
    if (window.getSelection()?.toString()) return;
    const href = topicHref(topic);
    if (e.metaKey || e.ctrlKey || e.button === 1) {
      window.open(href, "_blank", "noopener");
      return;
    }
    navigate(href);
  }

  const sortProps = { sortColumn, sortDir, onSort: handleSort };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Database className="h-5 w-5 text-muted-foreground" />
          <h1 className="text-2xl font-semibold tracking-tight">
            Topics
            {cluster && (
              <span className="text-muted-foreground font-normal inline-flex items-center">
                <span className="mx-1.5">—</span>
                {clusterInfo && <StatusDot status={clusterInfo.status} className="mr-1.5" />}
                {cluster}
                {clusterInfo?.paused && <span className="text-amber-500 text-sm ml-1.5">(paused)</span>}
              </span>
            )}
          </h1>
        </div>
        <div className="relative w-64">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search topics… (/ or ⌘K)"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setSearch("");
                e.currentTarget.blur();
              }
            }}
            className="pl-9 pr-8"
          />
          {search && (
            <button
              type="button"
              aria-label="Clear search"
              onClick={() => {
                setSearch("");
                searchRef.current?.focus();
              }}
              className="absolute right-2 top-2.5 text-muted-foreground hover:text-foreground cursor-pointer"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {!isLoading && !error && (
        <p className="text-sm text-muted-foreground">
          {search
            ? `${filteredTopics.length} of ${topics?.length ?? 0} topics`
            : `${filteredTopics.length} topics`}
          {" · "}{totals.messages.toLocaleString()} messages · {totals.keys.toLocaleString()} unique keys
          {totals.syncing > 0 && (
            <span className="text-blue-500"> · {totals.syncing} syncing</span>
          )}
        </p>
      )}

      {error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load topics: {(error as Error).message}
        </div>
      ) : isLoading ? (
        <TableSkeleton />
      ) : (
        <div className="rounded-lg border">
          <table ref={tableRef} className="w-full caption-bottom text-sm">
            <TableHeader>
              <TableRow>
                <SortableHead {...sortProps} column="name" label="Topic Name" className="w-[40%]" />
                <SortableHead {...sortProps} column="messages" label="Messages" className="text-right" />
                <SortableHead {...sortProps} column="keys" label="Unique Keys" className="text-right" />
                <TableHead className="w-[180px] text-muted-foreground">Progress</TableHead>
                <SortableHead {...sortProps} column="last_updated" label="Last Updated" className="text-right" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredTopics.length === 0 && (
                <TableRow className="hover:bg-transparent">
                  <TableCell colSpan={5} className="h-32 text-center text-muted-foreground space-y-1">
                    {search ? (
                      "No topics matching your search"
                    ) : (
                      <EmptyState
                        cluster={cluster}
                        clusterInfo={clusterInfo}
                        compactedOnly={settings?.config.compacted_only}
                      />
                    )}
                  </TableCell>
                </TableRow>
              )}
              {paddingTop > 0 && (
                <tr aria-hidden="true"><td colSpan={5} style={{ height: paddingTop }} /></tr>
              )}
              {visibleRows.map((topic) => (
                <TableRow
                  key={`${topic.cluster}/${topic.name}`}
                  className="cursor-pointer"
                  onClick={(e) => handleRowClick(e, topic)}
                  onAuxClick={(e) => e.button === 1 && handleRowClick(e, topic)}
                >
                  <TableCell className="font-mono text-sm font-medium">
                    <div className="flex items-center gap-2 flex-wrap">
                      <Link
                        to={topicHref(topic)}
                        title={`View ${topic.key_count.toLocaleString()} keys`}
                        className="rounded-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        {topic.name}
                      </Link>
                      {["json", "avro", "protobuf"].includes(topic.message_format?.toLowerCase()) && (
                        <Badge variant="outline" className={formatBadgeClass(topic.message_format)}>
                          {formatLabel(topic.message_format)}
                        </Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {topic.message_count.toLocaleString()}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {topic.key_count.toLocaleString()}
                  </TableCell>
                  <TableCell>
                    {isSyncing(topic) ? (
                      <span className="inline-flex items-center gap-2">
                        <Progress
                          value={topic.progress.percent}
                          className="h-2 w-16"
                          indicatorClassName="bg-blue-500"
                        />
                        <span className="text-xs tabular-nums text-muted-foreground">
                          {Math.round(topic.progress.percent)}% · {(topic.progress.total - topic.progress.consumed).toLocaleString()} left
                        </span>
                      </span>
                    ) : topic.progress.total === 0 ? (
                      <span className="text-xs text-muted-foreground">—</span>
                    ) : (
                      <Check className="h-4 w-4 text-muted-foreground/50" aria-label="Caught up" />
                    )}
                  </TableCell>
                  <TableCell className="text-right text-muted-foreground">
                    <span title={new Date(topic.last_updated).toLocaleString()}>
                      {timeAgo(new Date(topic.last_updated), now)}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
              {paddingBottom > 0 && (
                <tr aria-hidden="true"><td colSpan={5} style={{ height: paddingBottom }} /></tr>
              )}
            </TableBody>
          </table>
        </div>
      )}
    </div>
  );
}
