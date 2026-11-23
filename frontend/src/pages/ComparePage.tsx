import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { fetchClusters, fetchKeyHistory, type Message } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { JsonViewer } from "@/components/JsonViewer";
import { MessageDiff } from "@/components/MessageDiff";
import { ArrowLeftRight, GitCompare, Trash2 } from "lucide-react";
import { compareUrl, type CompareSide } from "@/lib/compare";

type SideName = "left" | "right";
const PREFIX: Record<SideName, string> = { left: "l", right: "r" };

function readSide(params: URLSearchParams, side: SideName): CompareSide {
  const p = PREFIX[side];
  return {
    cluster: params.get(`${p}c`) ?? "",
    topic: params.get(`${p}t`) ?? "",
    key: params.get(`${p}k`) ?? "",
  };
}

function isComplete(s: CompareSide) {
  return !!(s.cluster && s.topic && s.key);
}

function SideForm({
  label,
  value,
  clusters,
  onChange,
}: {
  label: string;
  value: CompareSide;
  clusters: string[];
  onChange: (v: CompareSide) => void;
}) {
  return (
    <Card className="p-4 space-y-2 flex-1 min-w-0">
      <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      <select
        aria-label={`${label} cluster`}
        value={value.cluster}
        onChange={(e) => onChange({ ...value, cluster: e.target.value })}
        className="h-8 w-full rounded-md border bg-background px-2 text-sm"
      >
        <option value="">Select cluster…</option>
        {clusters.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </select>
      <Input
        aria-label={`${label} topic`}
        placeholder="Topic"
        value={value.topic}
        onChange={(e) => onChange({ ...value, topic: e.target.value })}
        className="h-8 text-sm font-mono"
      />
      <Input
        aria-label={`${label} key`}
        placeholder="Key"
        value={value.key}
        onChange={(e) => onChange({ ...value, key: e.target.value })}
        className="h-8 text-sm font-mono"
      />
    </Card>
  );
}

function useLatest(side: CompareSide, enabled: boolean) {
  return useQuery({
    queryKey: ["compare-latest", side.cluster, side.topic, side.key],
    queryFn: async () => (await fetchKeyHistory(side.topic, side.key, side.cluster, 1, 1)).data[0] ?? null,
    enabled,
  });
}

function SidePanel({ side, message, isLoading }: { side: CompareSide; message: Message | null | undefined; isLoading: boolean }) {
  return (
    <Card className="flex-1 min-w-0">
      <CardHeader className="space-y-1 pb-3">
        <Link
          to={`/clusters/${encodeURIComponent(side.cluster)}/topics/${encodeURIComponent(side.topic)}/keys/${encodeURIComponent(side.key)}`}
          className="font-mono text-sm font-medium hover:underline break-all"
        >
          {side.key}
        </Link>
        <div className="flex flex-wrap gap-1.5">
          <Badge variant="secondary" className="text-xs">{side.cluster}</Badge>
          <Badge variant="outline" className="text-xs font-mono">{side.topic}</Badge>
          {message && (
            <Badge variant="outline" className="text-xs tabular-nums">
              p{message.partition} · offset {message.offset} · {new Date(message.timestamp).toLocaleString()}
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : message === null ? (
          <p className="text-sm text-muted-foreground">Key not found</p>
        ) : message?.body === null ? (
          <div className="flex items-center gap-2 rounded-lg bg-red-500/10 border border-red-500/20 p-4 text-sm text-red-500">
            <Trash2 className="h-4 w-4 shrink-0" />
            <span className="font-medium">Key deleted (tombstone)</span>
          </div>
        ) : message ? (
          <JsonViewer data={message.body} />
        ) : null}
      </CardContent>
    </Card>
  );
}

export function ComparePage() {
  const [params, setParams] = useSearchParams();
  const [draft, setDraft] = useState<Record<SideName, CompareSide>>(() => ({
    left: readSide(params, "left"),
    right: readSide(params, "right"),
  }));
  const left = readSide(params, "left");
  const right = readSide(params, "right");

  const { data: clusters } = useQuery({ queryKey: ["clusters"], queryFn: fetchClusters });
  const clusterNames = (clusters ?? []).map((c) => c.name).sort((a, b) => a.localeCompare(b));

  const ready = isComplete(left) && isComplete(right);
  const leftQuery = useLatest(left, ready);
  const rightQuery = useLatest(right, ready);

  const apply = (next: Record<SideName, CompareSide>) => {
    setDraft(next);
    setParams(new URL(compareUrl(next.left, next.right), window.location.origin).searchParams);
  };

  return (
    <div className="space-y-6 min-w-0">
      <div className="flex items-center gap-3">
        <GitCompare className="h-6 w-6 text-muted-foreground" />
        <h1 className="text-2xl font-semibold tracking-tight">Compare keys</h1>
      </div>
      <p className="text-sm text-muted-foreground">
        Diff the latest value of two keys — across topics or clusters (e.g. staging vs. production).
      </p>

      <form
        className="flex items-stretch gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          apply(draft);
        }}
      >
        <SideForm label="Left" value={draft.left} clusters={clusterNames} onChange={(v) => setDraft({ ...draft, left: v })} />
        <div className="flex flex-col justify-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            title="Swap sides"
            onClick={() => apply({ left: draft.right, right: draft.left })}
          >
            <ArrowLeftRight className="h-4 w-4" />
          </Button>
          <Button type="submit" size="sm" disabled={!isComplete(draft.left) || !isComplete(draft.right)}>
            Compare
          </Button>
        </div>
        <SideForm label="Right" value={draft.right} clusters={clusterNames} onChange={(v) => setDraft({ ...draft, right: v })} />
      </form>

      {ready && (
        <>
          {leftQuery.data && rightQuery.data && (
            <Card className="px-4 py-2">
              <MessageDiff prev={leftQuery.data} current={rightQuery.data} />
            </Card>
          )}
          {(leftQuery.error || rightQuery.error) && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
              Failed to load: {((leftQuery.error || rightQuery.error) as Error).message}
            </div>
          )}
          <div className="flex gap-3 items-start">
            <SidePanel side={left} message={leftQuery.data} isLoading={leftQuery.isLoading} />
            <SidePanel side={right} message={rightQuery.data} isLoading={rightQuery.isLoading} />
          </div>
        </>
      )}
    </div>
  );
}
