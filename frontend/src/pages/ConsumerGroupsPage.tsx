import { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchTopicConsumerGroups,
  fetchConsumerGroupLag,
  resetConsumerGroupOffsets,
} from "@/lib/api";
import type { ConsumerGroupInfo } from "@/lib/api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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
import { Input } from "@/components/ui/input";
import { Breadcrumbs } from "@/components/Breadcrumbs";
import { Users, ChevronDown, ChevronRight, RotateCcw, Loader2 } from "lucide-react";

const STATE_BADGE_CLASSES: Record<string, string> = {
  stable: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20",
  empty: "bg-amber-500/10 text-amber-500 border-amber-500/20",
};

function GroupRow({
  group,
  topic,
  cluster,
}: {
  group: ConsumerGroupInfo;
  topic: string;
  cluster?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const queryClient = useQueryClient();

  const { data: lagDetail, isLoading: isLagLoading } = useQuery({
    queryKey: ["consumer-group-lag", topic, group.group_id, cluster],
    queryFn: () => fetchConsumerGroupLag(topic, group.group_id, cluster),
    enabled: expanded,
  });

  const resetMutation = useMutation({
    mutationFn: ({ target, offsets }: { target: "earliest" | "latest" | "specific"; offsets?: Array<{ partition: number; offset: number }> }) =>
      resetConsumerGroupOffsets(topic, group.group_id, target, cluster, offsets),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["consumer-groups", topic, cluster] });
      queryClient.invalidateQueries({ queryKey: ["consumer-group-lag", topic, group.group_id, cluster] });
    },
  });

  const [specificOffsets, setSpecificOffsets] = useState<Record<number, string>>({});

  useEffect(() => {
    if (lagDetail) {
      setSpecificOffsets(
        Object.fromEntries(lagDetail.partitions.map(p => [p.partition, p.committed_offset.toString()]))
      );
    }
  }, [lagDetail]);

  const badgeClass =
    group.active_on_topic === 0
      ? "bg-amber-500/10 text-amber-500 border-amber-500/20"
      : STATE_BADGE_CLASSES[group.state.toLowerCase()] ??
        "bg-muted text-muted-foreground border-border";

  const badgeText = group.active_on_topic === 0 ? "Inactive" : group.state;

  const lagBadgeClass =
    group.total_lag === 0
      ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
      : group.total_lag > 10000
        ? "bg-red-500/10 text-red-500 border-red-500/20"
        : "bg-amber-500/10 text-amber-500 border-amber-500/20";

  return (
    <div className="border rounded-lg">
      <button
        type="button"
        className="flex w-full items-center gap-3 p-4 text-left hover:bg-muted/50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
        <span className="font-mono text-sm font-medium">
          {group.group_id}
        </span>
        <Badge variant="outline" className={badgeClass}>
          {badgeText}
        </Badge>
        <Badge variant="outline" className={lagBadgeClass}>
          {group.total_lag === 0
            ? "No lag"
            : group.total_lag.toLocaleString()}
        </Badge>
        <span className="ml-auto text-sm text-muted-foreground tabular-nums">
          {group.member_count} member{group.member_count !== 1 ? "s" : ""}
        </span>
      </button>
      {expanded && (
        <div className="border-t px-4 pb-4 pt-3 space-y-3">
          {isLagLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          ) : lagDetail ? (
            <>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <p className="text-sm text-muted-foreground">Total Lag</p>
                  <Badge
                    variant="outline"
                    className={
                      lagDetail.active_members === 0
                        ? "bg-amber-500/10 text-amber-500 border-amber-500/20"
                        : STATE_BADGE_CLASSES[lagDetail.state.toLowerCase()] ??
                          "bg-muted text-muted-foreground border-border"
                    }
                  >
                    {lagDetail.active_members === 0 ? "Inactive" : lagDetail.state}
                  </Badge>
                </div>
                <p className="text-lg font-semibold tabular-nums">
                  {lagDetail.total_lag.toLocaleString()}
                </p>
              </div>
              <div className="rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Partition</TableHead>
                      <TableHead className="text-right">
                        Committed Offset
                      </TableHead>
                      <TableHead className="text-right">End Offset</TableHead>
                      <TableHead className="text-right">Lag</TableHead>
                      <TableHead>Consumer</TableHead>
                      <TableHead>Host</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {lagDetail.partitions.map((p) => (
                      <TableRow key={p.partition}>
                        <TableCell className="tabular-nums">
                          {p.partition}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {p.committed_offset.toLocaleString()}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {p.end_offset.toLocaleString()}
                        </TableCell>
                        <TableCell className="text-right tabular-nums font-medium">
                          {p.lag.toLocaleString()}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {p.client_id || <span className="text-muted-foreground">&mdash;</span>}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {p.host || <span className="text-muted-foreground">&mdash;</span>}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              {lagDetail.active_members === 0 ? (
                <div className="flex items-center gap-2 pt-1">
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={resetMutation.isPending}
                      >
                        {resetMutation.isPending ? (
                          <Loader2 className="mr-2 h-3 w-3 animate-spin" />
                        ) : (
                          <RotateCcw className="mr-2 h-3 w-3" />
                        )}
                        Reset to Earliest
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Reset offsets to earliest?</AlertDialogTitle>
                        <AlertDialogDescription>
                          This will reset all committed offsets for {group.group_id} on
                          topic {topic} to earliest. This operation cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          onClick={() => resetMutation.mutate({ target: "earliest" })}
                        >
                          Reset to Earliest
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={resetMutation.isPending}
                      >
                        {resetMutation.isPending ? (
                          <Loader2 className="mr-2 h-3 w-3 animate-spin" />
                        ) : (
                          <RotateCcw className="mr-2 h-3 w-3" />
                        )}
                        Reset to Latest
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Reset offsets to latest?</AlertDialogTitle>
                        <AlertDialogDescription>
                          This will reset all committed offsets for {group.group_id} on
                          topic {topic} to latest. This operation cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          onClick={() => resetMutation.mutate({ target: "latest" })}
                        >
                          Reset to Latest
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                   </AlertDialog>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={resetMutation.isPending}
                      >
                        {resetMutation.isPending ? (
                          <Loader2 className="mr-2 h-3 w-3 animate-spin" />
                        ) : (
                          <RotateCcw className="mr-2 h-3 w-3" />
                        )}
                        Reset to Specific
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Reset offsets to specific values?</AlertDialogTitle>
                        <AlertDialogDescription>
                          Set the committed offset for each partition of {group.group_id} on
                          topic {topic}. This operation cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <div className="rounded-lg border">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>Partition</TableHead>
                              <TableHead className="text-right">Current Offset</TableHead>
                              <TableHead className="text-right">New Offset</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {lagDetail.partitions.map((p) => (
                              <TableRow key={p.partition}>
                                <TableCell className="tabular-nums">{p.partition}</TableCell>
                                <TableCell className="text-right tabular-nums">{p.committed_offset.toLocaleString()}</TableCell>
                                <TableCell className="text-right">
                                  <Input
                                    type="number"
                                    className="h-8 w-32 ml-auto tabular-nums text-right"
                                    value={specificOffsets[p.partition] ?? p.committed_offset.toString()}
                                    onChange={(e) =>
                                      setSpecificOffsets(prev => ({ ...prev, [p.partition]: e.target.value }))
                                    }
                                  />
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          onClick={() =>
                            resetMutation.mutate({
                              target: "specific",
                              offsets: lagDetail.partitions.map(p => ({
                                partition: p.partition,
                                offset: parseInt(specificOffsets[p.partition] ?? p.committed_offset.toString(), 10),
                              })),
                            })
                          }
                        >
                          Reset to Specific
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground pt-1">
                  Stop all consumers in the group (across all topics) to enable offset reset
                </p>
              )}
            </>
          ) : null}
        </div>
      )}
    </div>
  );
}

export function ConsumerGroupsPage() {
  const { cluster, topic } = useParams<{ cluster: string; topic: string }>();

  const {
    data: consumerGroups,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["consumer-groups", topic, cluster],
    queryFn: () => fetchTopicConsumerGroups(topic!, cluster),
    enabled: !!topic,
    refetchInterval: 5_000,
  });

  return (
    <div className="space-y-6">
      <Breadcrumbs
        items={[
          { label: cluster ? `Topics — ${cluster}` : "Topics", to: cluster ? `/clusters/${encodeURIComponent(cluster)}` : "/" },
          {
            label: decodeURIComponent(topic ?? ""),
            to: `/clusters/${encodeURIComponent(cluster!)}/topics/${encodeURIComponent(topic!)}/keys`,
          },
          { label: "Consumer Groups" },
        ]}
      />

      <div className="flex items-center gap-3">
        <Users className="h-6 w-6" />
        <h1 className="text-2xl font-semibold tracking-tight">
          Consumer Groups
        </h1>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load consumer groups: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <Card className="p-5 space-y-4">
          <Skeleton className="h-5 w-64" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </Card>
      ) : consumerGroups ? (
        <>
          <Card className="p-5">
            <p className="text-sm text-muted-foreground">
              {consumerGroups.length} consumer group
              {consumerGroups.length !== 1 ? "s" : ""} reading{" "}
              <span className="font-medium text-foreground">
                {decodeURIComponent(topic ?? "")}
              </span>
            </p>
          </Card>

          {consumerGroups.length > 0 ? (
            <div className="space-y-3">
              {consumerGroups.map((group) => (
                <GroupRow
                  key={group.group_id}
                  group={group}
                  topic={topic!}
                  cluster={cluster}
                />
              ))}
            </div>
          ) : (
            <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
              No consumer groups found for this topic.
            </div>
          )}
        </>
      ) : null}
    </div>
  );
}
