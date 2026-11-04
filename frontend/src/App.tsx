import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { fetchClusters } from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import { TopicsPage } from "@/pages/TopicsPage";
import { KeysPage } from "@/pages/KeysPage";
import { HistoryPage } from "@/pages/HistoryPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { ConsumerGroupsPage } from "@/pages/ConsumerGroupsPage";
import { Sidebar } from "@/components/Sidebar";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const clusterMatch = location.pathname.match(/^\/clusters\/([^/]+)/);
  const activeCluster = clusterMatch ? decodeURIComponent(clusterMatch[1]) : null;

  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar activeCluster={activeCluster} />
      <main className="flex-1 px-6 py-6">{children}</main>
    </div>
  );
}

function HomeRedirect() {
  const { data: clusters, isLoading } = useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
  });

  if (isLoading) return <Skeleton className="h-8 w-64" />;

  const first = clusters?.map((c) => c.name).sort((a, b) => a.localeCompare(b))[0];
  if (!first) return <TopicsPage />;

  return <Navigate to={`/clusters/${encodeURIComponent(first)}`} replace />;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<HomeRedirect />} />
            <Route path="/clusters/:cluster" element={<TopicsPage />} />
            <Route path="/clusters/:cluster/topics/:topic/keys" element={<KeysPage />} />
            <Route path="/clusters/:cluster/topics/:topic/keys/:key" element={<HistoryPage />} />
            <Route path="/clusters/:cluster/topics/:topic/consumer-groups" element={<ConsumerGroupsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
