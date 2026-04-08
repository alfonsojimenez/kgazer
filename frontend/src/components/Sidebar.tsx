import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { fetchClusters, fetchHealth, type ClusterInfo } from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import { Settings } from "lucide-react";

interface SidebarProps {
  activeCluster: string | null;
}

const STATUS_DOT: Record<ClusterInfo["status"], string> = {
  connected: "bg-[#22c55e]",
  connecting: "bg-[#f59e0b] animate-pulse",
  disconnected: "bg-[#ef4444]",
};

export function Sidebar({ activeCluster }: SidebarProps) {
  const navigate = useNavigate();
  const { data: clusters, isLoading } = useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
    refetchInterval: 10_000,
  });

  const { data: health } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
  });

  const sorted = useMemo(
    () => clusters ? [...clusters].sort((a, b) => a.name.localeCompare(b.name)) : [],
    [clusters],
  );

  const location = useLocation();

  useEffect(() => {
    if (!activeCluster && sorted.length > 0 && location.pathname === "/") {
      navigate(`/clusters/${encodeURIComponent(sorted[0].name)}`, { replace: true });
    }
  }, [activeCluster, sorted, navigate, location.pathname]);

  return (
    <aside className="w-56 shrink-0 border-r border-sidebar-border bg-sidebar-background sticky top-0 h-screen overflow-y-auto flex flex-col">
      <div className="px-4 pt-4 pb-4">
        <Link to="/" className="block">
          <img src="/logo.png" alt="KGazer" className="h-20 object-contain" />
        </Link>
      </div>

      <div className="px-4 pb-2">
        <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Clusters
        </h2>
      </div>

      <nav className="px-2 pb-4 space-y-0.5 flex-1">
        {isLoading && (
          <div className="space-y-1.5 px-2 pt-1">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-7 w-full" />
            ))}
          </div>
        )}

        {sorted.map((cluster) => (
          <button
            key={cluster.name}
            onClick={() => {
              navigate(`/clusters/${encodeURIComponent(cluster.name)}`);
            }}
            className={`w-full flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors cursor-pointer ${
              activeCluster === cluster.name
                ? "bg-primary/10 text-primary font-medium"
                : "text-sidebar-foreground hover:bg-sidebar-accent/50"
            }`}
          >
            <span
              className={`h-2 w-2 shrink-0 rounded-full ${STATUS_DOT[cluster.status]}`}
            />
            {cluster.name}
          </button>
        ))}
      </nav>

      <div className="border-t border-sidebar-border px-2 py-3 mt-auto">
        <Link
          to="/settings"
          className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-sidebar-foreground hover:bg-sidebar-accent/50 transition-colors"
        >
          <Settings className="h-4 w-4 text-muted-foreground" />
          Settings
        </Link>
        <div className="flex items-center justify-between px-2 pt-2">
          {health?.version && (
            <span className="text-[10px] text-muted-foreground/50">v{health.version}</span>
          )}
          <a
            href="https://github.com/alfonsojimenez/kgazer"
            target="_blank"
            rel="noopener noreferrer"
            className="text-muted-foreground/40 hover:text-muted-foreground transition-colors"
          >
            <svg viewBox="0 0 16 16" fill="currentColor" className="h-3.5 w-3.5"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
          </a>
        </div>
      </div>
    </aside>
  );
}
