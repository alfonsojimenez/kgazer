import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchOrphanedClusters, deleteClusterData, fetchSettingsInfo } from "@/lib/api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
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
import { Activity, Database, Server, Settings, Trash2 } from "lucide-react";

function TableSkeleton() {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: 4 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}

export function SettingsPage() {
  const queryClient = useQueryClient();

  const { data: info } = useQuery({
    queryKey: ["settings-info"],
    queryFn: fetchSettingsInfo,
  });

  const { data: clusters, isLoading, error } = useQuery({
    queryKey: ["orphaned-clusters"],
    queryFn: fetchOrphanedClusters,
  });

  const deleteMutation = useMutation({
    mutationFn: deleteClusterData,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orphaned-clusters"] });
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Settings className="h-5 w-5 text-muted-foreground" />
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
      </div>

      {info && (
        <div className="space-y-4">
          <div>
            <h2 className="text-lg font-medium flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              Runtime
            </h2>
          </div>
          <Card className="p-5">
            <div className="grid grid-cols-2 gap-x-8 gap-y-3 sm:grid-cols-4 text-sm">
              <div>
                <span className="text-muted-foreground">Version</span>
                <p className="font-semibold">{info.version}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Uptime</span>
                <p className="font-semibold">{info.uptime}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Total Topics</span>
                <p className="font-semibold tabular-nums">{info.stats.total_topics.toLocaleString()}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Total Messages</span>
                <p className="font-semibold tabular-nums">{info.stats.total_messages.toLocaleString()}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Total Keys</span>
                <p className="font-semibold tabular-nums">{info.stats.total_keys.toLocaleString()}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Compacted Only</span>
                <p className="font-semibold">{info.config.compacted_only ? "Yes" : "No"}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Server Port</span>
                <p className="font-semibold tabular-nums">{info.config.server_port}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Database</span>
                <p className="font-semibold">{info.config.db_host} / {info.config.db_name}</p>
              </div>
            </div>
          </Card>
        </div>
      )}

      {info && (
        <div className="space-y-4">
          <div>
            <h2 className="text-lg font-medium flex items-center gap-2">
              <Server className="h-4 w-4 text-muted-foreground" />
              Cluster Configuration
            </h2>
          </div>
          <div className="space-y-3">
            {info.clusters.map((cluster) => {
              const status = info.cluster_status.find(s => s.name === cluster.name);
              return (
                <Card key={cluster.name} className="p-4">
                  <div className="flex items-center gap-3 mb-2">
                    <span className={`h-2 w-2 rounded-full ${status?.status === "connected" ? "bg-emerald-500" : status?.status === "connecting" ? "bg-amber-500 animate-pulse" : "bg-red-500"}`} />
                    <span className="font-semibold text-sm">{cluster.name}</span>
                    {status?.paused && <Badge variant="outline" className="bg-amber-500/10 text-amber-500 border-amber-500/20 text-[10px]">Paused</Badge>}
                  </div>
                  <div className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm text-muted-foreground">
                    <div>Bootstrap: <span className="font-mono text-foreground text-xs">{cluster.bootstrap_servers}</span></div>
                    <div>Schema Registry: <span className="text-foreground">{cluster.schema_registry ? "Configured" : "None"}</span></div>
                    {cluster.properties && Object.entries(cluster.properties).map(([k, v]) => (
                      <div key={k}>{k}: <span className="font-mono text-foreground text-xs">{v}</span></div>
                    ))}
                  </div>
                </Card>
              );
            })}
          </div>
        </div>
      )}

      <div className="space-y-4">
        <div>
          <h2 className="text-lg font-medium flex items-center gap-2">
            <Database className="h-4 w-4 text-muted-foreground" />
            Orphaned Clusters
          </h2>
          <p className="text-sm text-muted-foreground">
            Clusters with data in the database that are no longer present in the configuration.
          </p>
        </div>

        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
            Failed to load orphaned clusters: {(error as Error).message}
          </div>
        )}

        {isLoading ? (
          <TableSkeleton />
        ) : (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[40%]">Cluster Name</TableHead>
                  <TableHead className="text-right">Topics</TableHead>
                  <TableHead className="text-right">Messages</TableHead>
                  <TableHead className="text-right">Keys</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {clusters?.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                      No orphaned clusters found. All clusters in the database match the current configuration.
                    </TableCell>
                  </TableRow>
                )}
                {clusters?.map((cluster) => (
                  <TableRow key={cluster.name}>
                    <TableCell className="font-mono text-sm font-medium">
                      {cluster.name}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {cluster.topic_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <Badge variant="secondary" className="tabular-nums">
                        {cluster.message_count.toLocaleString()}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {cluster.key_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={deleteMutation.isPending}
                          >
                            <Trash2 className="h-4 w-4" />
                            Delete Data
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Delete cluster data</AlertDialogTitle>
                            <AlertDialogDescription>
                              This will permanently delete all topics, messages, and keys for
                              cluster "{cluster.name}". This action cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              onClick={() => deleteMutation.mutate(cluster.name)}
                              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                            >
                              Delete
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  );
}
