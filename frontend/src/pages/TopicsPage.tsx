import { useState, useMemo, useRef, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { fetchTopics, fetchClusters } from "@/lib/api";
import type { ClusterInfo } from "@/lib/api";
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
import { ArrowDown, ArrowUp, Check, Database, Search } from "lucide-react";

function TableSkeleton() {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: 6 }).map((_, i) => (
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

function formatLabel(format: string): string {
  const labels: Record<string, string> = { json: "JSON", avro: "Avro" };
  return labels[format?.toLowerCase()] ?? "Other";
}

function formatBadgeClass(format: string): string {
  const classes: Record<string, string> = {
    json: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20 text-[10px] px-1.5 py-0",
    avro: "bg-blue-500/10 text-blue-500 border-blue-500/20 text-[10px] px-1.5 py-0",
  };
  return classes[format?.toLowerCase()] ?? "text-[10px] px-1.5 py-0";
}

export function TopicsPage() {
  const navigate = useNavigate();
  const { cluster } = useParams<{ cluster: string }>();

  const [sortColumn, setSortColumn] = useState<"name" | "last_updated">("last_updated");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [search, setSearch] = useState("");

  const searchRef = useRef<HTMLInputElement>(null);

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

  const { data: topics, isLoading, error } = useQuery({
    queryKey: ["topics", cluster],
    queryFn: () => fetchTopics(cluster),
  });

  const { data: clusters } = useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
  });

  const clusterInfo = clusters?.find((c: ClusterInfo) => c.name === cluster);

  const filteredTopics = useMemo(() => {
    if (!topics) return [];
    let filtered = topics;
    if (search) {
      const lower = search.toLowerCase();
      filtered = filtered.filter((t) => t.name.toLowerCase().includes(lower));
    }
    return [...filtered].sort((a, b) => {
      let cmp: number;
      if (sortColumn === "name") {
        cmp = a.name.localeCompare(b.name);
      } else {
        cmp = new Date(a.last_updated).getTime() - new Date(b.last_updated).getTime();
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [topics, search, sortColumn, sortDir]);

  const totalMessages = useMemo(
    () => filteredTopics.reduce((sum, t) => sum + t.message_count, 0),
    [filteredTopics]
  );

  const totalKeys = useMemo(
    () => filteredTopics.reduce((sum, t) => sum + t.key_count, 0),
    [filteredTopics]
  );

  function handleSort(column: "name" | "last_updated") {
    if (sortColumn === column) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortColumn(column);
      setSortDir(column === "name" ? "asc" : "desc");
    }
  }

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
                {clusterInfo && (
                  <span className={`inline-block h-2 w-2 rounded-full mr-1.5 ${
                    clusterInfo.status === "connected" ? "bg-emerald-500" :
                    clusterInfo.status === "connecting" ? "bg-amber-500 animate-pulse" :
                    "bg-red-500"
                  }`} />
                )}
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
            className="pl-9"
          />
        </div>
      </div>

      {!isLoading && (
        <p className="text-sm text-muted-foreground">
          {search
            ? `${filteredTopics.length} of ${topics?.length ?? 0} topics`
            : `${filteredTopics.length} topics`}
          {" · "}{totalMessages.toLocaleString()} messages · {totalKeys.toLocaleString()} unique keys
        </p>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load topics: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <TableSkeleton />
      ) : (
        <div className="rounded-lg border">
          <table className="w-full caption-bottom text-sm">
            <TableHeader>
              <TableRow>
                <TableHead
                  className={`w-[40%] cursor-pointer select-none hover:text-foreground  ${sortColumn === "name" ? "font-semibold text-foreground" : "text-muted-foreground"}`}
                  onClick={() => handleSort("name")}
                >
                  <span className="inline-flex items-center gap-1">
                    Topic Name
                    {sortColumn === "name" && (sortDir === "desc" ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />)}
                  </span>
                </TableHead>
                <TableHead className="text-right  text-muted-foreground">Messages</TableHead>
                <TableHead className="text-right  text-muted-foreground">Unique Keys</TableHead>
                <TableHead className="w-[140px]  text-muted-foreground">Progress</TableHead>
                <TableHead
                  className={`text-right cursor-pointer select-none hover:text-foreground  ${sortColumn === "last_updated" ? "font-semibold text-foreground" : "text-muted-foreground"}`}
                  onClick={() => handleSort("last_updated")}
                >
                  <span className="inline-flex items-center gap-1">
                    Last Updated
                    {sortColumn === "last_updated" && (sortDir === "desc" ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />)}
                  </span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredTopics.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                    {search ? "No topics matching your search" : "No topics found"}
                  </TableCell>
                </TableRow>
              )}
              {filteredTopics.map((topic) => (
                <TableRow
                  key={topic.name}
                  className="cursor-pointer"
                  title={`View ${topic.key_count.toLocaleString()} keys`}
                  onClick={() => navigate(`/clusters/${encodeURIComponent(topic.cluster)}/topics/${encodeURIComponent(topic.name)}/keys`)}
                >
                  <TableCell className="font-mono text-sm font-medium">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span>{topic.name}</span>
                      {["json", "avro"].includes(topic.message_format?.toLowerCase()) && (
                        <Badge variant="outline" className={formatBadgeClass(topic.message_format)}>
                          {formatLabel(topic.message_format)}
                        </Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <Badge variant="secondary" className="tabular-nums">
                      {topic.message_count.toLocaleString()}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {topic.key_count.toLocaleString()}
                  </TableCell>
                  <TableCell>
                    {topic.progress.total === 0 ? (
                      <span className="text-xs text-muted-foreground">—</span>
                    ) : topic.progress.done || topic.progress.percent >= 99.9 ? (
                      <span className="inline-flex items-center gap-1.5">
                        <Check className="h-4 w-4 text-emerald-500" />
                        <span className="text-xs text-emerald-500">Done</span>
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-2">
                        <Progress
                          value={topic.progress.percent}
                          className="h-2 w-16 animate-pulse"
                          indicatorClassName="bg-blue-500"
                        />
                        <span className="text-xs tabular-nums text-muted-foreground">
                          {Math.round(topic.progress.percent)}% · {(topic.progress.total - topic.progress.consumed).toLocaleString()} left
                        </span>
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-right text-muted-foreground">
                    <span title={new Date(topic.last_updated).toLocaleString()}>
                      {timeAgo(new Date(topic.last_updated))}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </table>
        </div>
      )}
    </div>
  );
}
