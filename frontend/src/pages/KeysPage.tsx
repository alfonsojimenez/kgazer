import { useState, useEffect, useCallback, useRef } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchKeys,
  fetchTopicDetail,
  reconsumeTopics,

  fetchTopicConsumerGroups,
} from "@/lib/api";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Breadcrumbs } from "@/components/Breadcrumbs";
import { Pagination } from "@/components/Pagination";
import { Progress } from "@/components/ui/progress";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { Search, RotateCcw, Loader2, ArrowDown, ArrowUp, Check, Users, Filter, X } from "lucide-react";

const FORMAT_BADGE_CLASSES: Record<string, string> = {
  json: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20",
  avro: "bg-blue-500/10 text-blue-500 border-blue-500/20",
};

const COMPACTED_BADGE_CLASS = "bg-violet-500/10 text-violet-500 border-violet-500/20";

function formatLabel(format: string): string {
  const labels: Record<string, string> = { json: "JSON", avro: "Avro" };
  return labels[format.toLowerCase()] ?? "Other";
}

function TopicDetailSkeleton() {
  return (
    <Card className="p-5 space-y-4">
      <Skeleton className="h-7 w-64" />
      <Skeleton className="h-5 w-full max-w-md" />
      <Skeleton className="h-5 w-48" />
    </Card>
  );
}

function highlightMatch(text: string, search: string) {
  const idx = text.toLowerCase().indexOf(search.toLowerCase());
  if (idx === -1) return text;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="bg-yellow-200 dark:bg-yellow-500/30 rounded-sm px-0.5">{text.slice(idx, idx + search.length)}</mark>
      {text.slice(idx + search.length)}
    </>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: 8 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
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

export function KeysPage() {
  const { cluster, topic } = useParams<{ cluster: string; topic: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(1);
  const [sortBy, setSortBy] = useState<"last_updated" | "offset">("last_updated");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [selectedPartitions, setSelectedPartitions] = useState<number[]>([]);
  const [offsetFilter, setOffsetFilter] = useState("");
  const [debouncedOffset, setDebouncedOffset] = useState(0);
  const [partitionPopoverOpen, setPartitionPopoverOpen] = useState(false);
  const [reconsumeOpen, setReconsumeOpen] = useState(false);

  const searchRef = useRef<HTMLInputElement>(null);
  const limit = 25;

  const debounce = useCallback(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    return debounce();
  }, [debounce]);

  useEffect(() => {
    const timer = setTimeout(() => {
      const val = parseInt(offsetFilter, 10);
      setDebouncedOffset(isNaN(val) ? 0 : val);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [offsetFilter]);

  useEffect(() => {
    setPage(1);
  }, [selectedPartitions]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "/" && !e.metaKey && !e.ctrlKey && document.activeElement?.tagName !== "INPUT") {
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

  const activeFilterCount =
    (search ? 1 : 0) +
    (selectedPartitions.length > 0 ? 1 : 0) +
    (offsetFilter ? 1 : 0);

  const { data: topicDetail, isLoading: isDetailLoading } = useQuery({
    queryKey: ["topic-detail", topic, cluster],
    queryFn: () => fetchTopicDetail(topic!, cluster),
    enabled: !!topic,
    refetchInterval: 5_000,
    refetchIntervalInBackground: true,
  });

  const { data, isLoading, error } = useQuery({
    queryKey: ["keys", topic, cluster, debouncedSearch, sortBy, sortDir, page, selectedPartitions, debouncedOffset],
    queryFn: () => fetchKeys(topic!, cluster, debouncedSearch || undefined, sortBy, sortDir, page, limit, selectedPartitions.length > 0 ? selectedPartitions : undefined, debouncedOffset || undefined),
    enabled: !!topic,
    refetchInterval: 5_000,
    refetchIntervalInBackground: true,
  });

  const { data: consumerGroups } = useQuery({
    queryKey: ["consumer-groups", topic, cluster],
    queryFn: () => fetchTopicConsumerGroups(topic!, cluster),
    enabled: !!topic,
  });

  const reconsumeMutation = useMutation({
    mutationFn: () => reconsumeTopics(topic!, cluster),
    onSuccess: () => {
      setReconsumeOpen(false);
      queryClient.invalidateQueries({ queryKey: ["topic-detail", topic] });
      queryClient.invalidateQueries({ queryKey: ["keys", topic] });
    },
  });

  const formatKey = topicDetail?.message_format?.toLowerCase() ?? "unknown";
  const formatBadgeClass = FORMAT_BADGE_CLASSES[formatKey];

  return (
    <div className="space-y-6">
      <Breadcrumbs
        items={[
          { label: cluster ? `Topics — ${cluster}` : "Topics", to: cluster ? `/clusters/${encodeURIComponent(cluster)}` : "/" },
          { label: decodeURIComponent(topic ?? "") },
        ]}
      />

      {isDetailLoading ? (
        <TopicDetailSkeleton />
      ) : topicDetail ? (
        <Card className="p-5 space-y-4">
          <div className="flex items-center gap-3 flex-wrap">
            <h2 className="text-xl font-semibold tracking-tight">
              {topicDetail.name}
            </h2>
            <Badge
              variant={formatBadgeClass ? "outline" : "secondary"}
              className={formatBadgeClass}
            >
              {formatLabel(topicDetail.message_format)}
            </Badge>
            {topicDetail.compacted && (
              <Badge variant="outline" className={COMPACTED_BADGE_CLASS}>
                Compacted
              </Badge>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
            <div>
              <p className="text-muted-foreground text-xs uppercase tracking-wide">Cluster</p>
              <p className="text-lg font-semibold tabular-nums">{topicDetail.cluster}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs uppercase tracking-wide">Partitions</p>
              <p className="text-lg font-semibold tabular-nums">{topicDetail.partitions}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs uppercase tracking-wide">Consumed Messages</p>
              <p className="text-lg font-semibold tabular-nums">{topicDetail.consumed_messages.toLocaleString()}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs uppercase tracking-wide">Unique Keys</p>
              <p className="text-lg font-semibold tabular-nums">{topicDetail.unique_keys.toLocaleString()}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs uppercase tracking-wide">Progress</p>
              {topicDetail.progress.total === 0 ? (
                <p className="text-lg font-semibold text-muted-foreground">Waiting…</p>
              ) : topicDetail.progress.done || topicDetail.progress.percent >= 99.9 ? (
                <div className="space-y-1 pt-1">
                  <Progress
                    value={100}
                    className="h-2.5"
                    indicatorClassName="bg-emerald-500"
                  />
                  <p className="flex items-center gap-1.5 text-xs font-semibold text-emerald-500">
                    <Check className="h-3.5 w-3.5" />
                    Complete — {topicDetail.consumed_messages.toLocaleString()} messages stored
                  </p>
                </div>
              ) : (
                <div className="space-y-1 pt-1">
                  <Progress
                    value={topicDetail.progress.percent}
                    className="h-2.5 animate-pulse"
                    indicatorClassName="bg-blue-500"
                  />
                  <p className="text-xs tabular-nums text-muted-foreground">
                    {Math.round(topicDetail.progress.percent)}% — {topicDetail.consumed_messages.toLocaleString()} messages stored
                  </p>
                </div>
              )}
            </div>
          </div>

          <div className="h-5">
            {consumerGroups && consumerGroups.length > 0 && (
              <Link
                to={`/clusters/${encodeURIComponent(cluster!)}/topics/${encodeURIComponent(topic!)}/consumer-groups`}
                className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              >
                <Users className="h-4 w-4" />
                <span>{consumerGroups.length} consumer group{consumerGroups.length !== 1 ? "s" : ""} reading this topic</span>
              </Link>
            )}
          </div>

          <div className="flex items-center justify-end gap-2">
            <AlertDialog open={reconsumeOpen} onOpenChange={setReconsumeOpen}>
              <AlertDialogTrigger asChild>
                <Button variant="outline" size="sm">
                  <RotateCcw className="h-3.5 w-3.5" />
                  Re-consume from beginning
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Re-consume from beginning?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will delete all {topicDetail.consumed_messages.toLocaleString()} stored
                    messages for this topic and re-consume from the beginning. This action cannot be
                    undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    disabled={reconsumeMutation.isPending}
                    onClick={(e) => {
                      e.preventDefault();
                      reconsumeMutation.mutate();
                    }}
                  >
                    {reconsumeMutation.isPending && (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    )}
                    Yes, re-consume
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </Card>
      ) : null}

      <Separator />

      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-2xl font-semibold tracking-tight">Keys</h1>
        {activeFilterCount > 0 && (
          <Badge variant="secondary" className="text-xs">
            {activeFilterCount} filter{activeFilterCount > 1 ? "s" : ""}
          </Badge>
        )}
        <div className="relative ml-auto w-64">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search keys… (/ or ⌘K)"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
        <Popover open={partitionPopoverOpen} onOpenChange={setPartitionPopoverOpen}>
          <PopoverTrigger asChild>
            <Button variant="outline" size="sm">
              <Filter className="h-3.5 w-3.5" />
              {selectedPartitions.length > 0
                ? `Partitions (${selectedPartitions.length})`
                : "Partitions"}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-48 p-3">
            <div className="max-h-48 space-y-1.5 overflow-y-auto">
              {topicDetail &&
                Array.from({ length: topicDetail.partitions }, (_, i) => (
                  <label key={i} className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded border-gray-300"
                      checked={selectedPartitions.includes(i)}
                      onChange={() =>
                        setSelectedPartitions((prev) =>
                          prev.includes(i) ? prev.filter((p) => p !== i) : [...prev, i]
                        )
                      }
                    />
                    Partition {i}
                  </label>
                ))}
            </div>
            {selectedPartitions.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="mt-2 w-full"
                onClick={() => setSelectedPartitions([])}
              >
                Clear
              </Button>
            )}
          </PopoverContent>
        </Popover>
        <Input
          placeholder="Offset…"
          value={offsetFilter}
          onChange={(e) => setOffsetFilter(e.target.value)}
          className="w-32 h-8 text-xs"
        />
        {activeFilterCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSearch("");
              setSelectedPartitions([]);
              setOffsetFilter("");
            }}
          >
            <X className="h-3.5 w-3.5" />
            Clear all
          </Button>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load keys: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <TableSkeleton />
      ) : (
        <>
          <div className="rounded-lg border max-h-[calc(100vh-24rem)] overflow-auto">
            <table className="w-full caption-bottom text-sm border-separate border-spacing-0">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[40%] sticky top-0 bg-background z-10 border-b">Key</TableHead>
                  <TableHead className="text-right sticky top-0 bg-background z-10 border-b">Messages</TableHead>
                  <TableHead className="text-right sticky top-0 bg-background z-10 border-b">Partition</TableHead>
                  <TableHead
                    className="text-right cursor-pointer select-none hover:text-foreground sticky top-0 bg-background z-10 border-b"
                    onClick={() => {
                      if (sortBy === "offset") {
                        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
                      } else {
                        setSortBy("offset");
                        setSortDir("desc");
                      }
                    }}
                  >
                    <span className="inline-flex items-center gap-1">
                      Offset
                      {sortBy === "offset" && (
                        sortDir === "desc" ? (
                          <ArrowDown className="h-3 w-3" />
                        ) : (
                          <ArrowUp className="h-3 w-3" />
                        )
                      )}
                    </span>
                  </TableHead>
                  <TableHead
                    className="text-right cursor-pointer select-none hover:text-foreground sticky top-0 bg-background z-10 border-b"
                    onClick={() => {
                      if (sortBy === "last_updated") {
                        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
                      } else {
                        setSortBy("last_updated");
                        setSortDir("desc");
                      }
                    }}
                  >
                    <span className="inline-flex items-center gap-1">
                      Last Updated
                      {sortBy === "last_updated" && (
                        sortDir === "desc" ? (
                          <ArrowDown className="h-3 w-3" />
                        ) : (
                          <ArrowUp className="h-3 w-3" />
                        )
                      )}
                    </span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.data.length === 0 && !isLoading && (
                  <TableRow>
                    <TableCell colSpan={5} className="h-32 text-center">
                      {debouncedSearch ? (
                        <span className="text-muted-foreground">No keys match your search</span>
                      ) : (
                        <div className="flex flex-col items-center gap-3">
                          <div className="flex items-center gap-1.5">
                            <span className="h-1.5 w-1.5 rounded-full bg-foreground/60 animate-bounce [animation-delay:-0.3s]" />
                            <span className="h-1.5 w-1.5 rounded-full bg-foreground/60 animate-bounce [animation-delay:-0.15s]" />
                            <span className="h-1.5 w-1.5 rounded-full bg-foreground/60 animate-bounce" />
                          </div>
                          <span className="text-sm text-muted-foreground">Waiting for messages…</span>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                )}
                {data?.data.map((k) => (
                  <TableRow
                    key={k.key}
                    className="cursor-pointer"
                    onClick={() =>
                      navigate(
                        `/clusters/${encodeURIComponent(cluster!)}/topics/${encodeURIComponent(topic!)}/keys/${encodeURIComponent(k.key)}`
                      )
                    }
                  >
                     <TableCell className="font-mono text-sm font-medium">
                      {debouncedSearch ? highlightMatch(k.key, debouncedSearch) : k.key}
                    </TableCell>
                    <TableCell className="text-right">
                      <Badge variant="secondary" className="tabular-nums">
                        {k.message_count.toLocaleString()}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {k.partition}
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {k.offset.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      <span title={new Date(k.last_updated).toLocaleString()}>
                        {timeAgo(new Date(k.last_updated))}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </table>
          </div>
          {data && (
            <Pagination
              page={page}
              total={data.total}
              limit={limit}
              onPageChange={setPage}
            />
          )}
        </>
      )}
    </div>
  );
}
