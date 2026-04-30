import { useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { fetchKeyHistory, fetchKeyTimeline } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Breadcrumbs } from "@/components/Breadcrumbs";
import { DiffTimeline } from "@/components/DiffTimeline";
import { Pagination } from "@/components/Pagination";
import { JsonViewer } from "@/components/JsonViewer";
import { MessageDiff } from "@/components/MessageDiff";
import { Clock, Hash, Layers, Trash2 } from "lucide-react";

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

function HistorySkeleton() {
  return (
    <div className="space-y-4">
      {Array.from({ length: 3 }).map((_, i) => (
        <Skeleton key={i} className="h-48 w-full rounded-xl" />
      ))}
    </div>
  );
}

export function HistoryPage() {
  const { cluster, topic, key } = useParams<{ cluster: string; topic: string; key: string }>();
  const [page, setPage] = useState(1);
  const [allExpanded, setAllExpanded] = useState(false);
  const limit = 20;

  const { data, isLoading, error } = useQuery({
    queryKey: ["history", topic, key, page],
    queryFn: () => fetchKeyHistory(topic!, key!, cluster, page, limit),
    enabled: !!topic && !!key,
    refetchInterval: 5_000,
  });

  const { data: timeline } = useQuery({
    queryKey: ["timeline", topic, key, cluster],
    queryFn: () => fetchKeyTimeline(topic!, key!, cluster),
    enabled: !!topic && !!key,
    staleTime: 30_000,
  });

  const decodedTopic = decodeURIComponent(topic ?? "");
  const decodedKey = decodeURIComponent(key ?? "");

  return (
    <div className="space-y-6 min-w-0">
      <Breadcrumbs
        items={[
          { label: cluster ? `Topics — ${cluster}` : "Topics", to: cluster ? `/clusters/${encodeURIComponent(cluster)}` : "/" },
          {
            label: decodedTopic,
            to: `/clusters/${encodeURIComponent(cluster!)}/topics/${encodeURIComponent(decodedTopic)}/keys`,
          },
          { label: decodedKey },
        ]}
      />

      <div className="flex items-center gap-3">
        <h1 className="text-2xl font-semibold tracking-tight font-mono">
          {decodedKey}
        </h1>
        {data && (
          <Badge variant="secondary" className="tabular-nums">
            {data.total} version{data.total !== 1 ? "s" : ""}
          </Badge>
        )}
        {data && data.data.length > 1 && (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-xs text-muted-foreground"
            onClick={() => setAllExpanded(!allExpanded)}
          >
            {allExpanded ? "Collapse all JSON" : "Expand all JSON"}
          </Button>
        )}
      </div>

      {data && data.data.length > 0 && (
        <Card className="p-4">
          <div className="flex items-center gap-6 text-sm flex-wrap">
            <div>
              <span className="text-muted-foreground">Versions</span>
              <span className="ml-1.5 font-semibold tabular-nums">{data.total}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Partition</span>
              <span className="ml-1.5 font-semibold tabular-nums">{data.data[0].partition}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Latest offset</span>
              <span className="ml-1.5 font-semibold tabular-nums">{data.data[0].offset.toLocaleString()}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Last updated</span>
              <span className="ml-1.5 font-semibold" title={new Date(data.data[0].timestamp).toLocaleString()}>
                {timeAgo(new Date(data.data[0].timestamp))}
              </span>
            </div>
          </div>
        </Card>
      )}

      {timeline && timeline.length > 1 && (
        <DiffTimeline
          points={timeline}
          onSelectVersion={(offset) => {
            const idx = timeline.findIndex((p) => p.offset === offset);
            if (idx === -1) return;
            const positionFromEnd = timeline.length - 1 - idx;
            const targetPage = Math.floor(positionFromEnd / limit) + 1;
            setPage(targetPage);
          }}
        />
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load history: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <HistorySkeleton />
      ) : (
        <>
          <div className="space-y-3">
            {data?.data.map((msg, i) => (
              <div key={msg.id}>
                <Card className={i === 0 && page === 1 ? "border-l-4 border-l-emerald-500" : ""}>
                  <CardHeader className="flex flex-row items-center gap-4 space-y-0 pb-3">
                    <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                      <Clock className="h-3.5 w-3.5" />
                      <span title={new Date(msg.timestamp).toLocaleString()}>
                        {timeAgo(new Date(msg.timestamp))}
                      </span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Hash className="h-3.5 w-3.5 text-muted-foreground" />
                      <Badge variant="outline" className="tabular-nums text-xs">
                        offset {msg.offset}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Layers className="h-3.5 w-3.5 text-muted-foreground" />
                      <Badge variant="outline" className="tabular-nums text-xs">
                        partition {msg.partition}
                      </Badge>
                    </div>
                    {i === 0 && page === 1 && (
                      <Badge className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20" variant="outline">
                        Latest
                      </Badge>
                    )}
                  </CardHeader>
                  <CardContent>
                    {msg.body === null ? (
                      <div className="flex items-center gap-2 rounded-lg bg-red-500/10 border border-red-500/20 p-4 text-sm text-red-500">
                        <Trash2 className="h-4 w-4 shrink-0" />
                        <span className="font-medium">Key deleted (tombstone)</span>
                      </div>
                    ) : (
                      <JsonViewer data={msg.body} forceExpanded={allExpanded} />
                    )}
                  </CardContent>
                </Card>

                {i < (data?.data.length ?? 0) - 1 && data?.data[i + 1] && (
                  <div className="flex items-center gap-2 py-1 pl-4">
                    <Separator className="flex-1" />
                    <MessageDiff current={msg} prev={data.data[i + 1]} />
                    <Separator className="flex-1" />
                  </div>
                )}
              </div>
            ))}
          </div>

          {data?.data.length === 0 && (
            <div className="flex h-40 items-center justify-center text-muted-foreground">
              No messages found for this key
            </div>
          )}

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
