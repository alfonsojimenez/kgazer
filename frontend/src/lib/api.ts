const API_BASE = import.meta.env.VITE_API_URL || "";

export interface Progress {
  total: number;
  consumed: number;
  percent: number;
  done: boolean;
}

export interface TopicSummary {
  name: string;
  cluster: string;
  partitions: number;
  compacted: boolean;
  message_format: string;
  message_count: number;
  key_count: number;
  last_updated: string;
  progress: Progress;
}

export interface KeySummary {
  key: string;
  message_count: number;
  partition: number;
  offset: number;
  last_updated: string;
}

export interface Message {
  id: number;
  key: string;
  body: unknown;
  partition: number;
  offset: number;
  timestamp: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
}

async function request<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`);
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export interface ClusterInfo {
  name: string;
  status: "connected" | "connecting" | "disconnected";
  paused: boolean;
}

export async function pauseCluster(cluster: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/clusters/${encodeURIComponent(cluster)}/pause`, { method: "POST" });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
}

export async function resumeCluster(cluster: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/clusters/${encodeURIComponent(cluster)}/resume`, { method: "POST" });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
}

export async function fetchHealth(): Promise<{ status: string; version: string }> {
  return request<{ status: string; version: string }>("/api/health");
}

export async function fetchClusters(): Promise<ClusterInfo[]> {
  return request<ClusterInfo[]>("/api/clusters");
}

export async function fetchTopics(cluster?: string, sort?: string): Promise<TopicSummary[]> {
  const params = new URLSearchParams();
  if (cluster) params.set("cluster", cluster);
  if (sort) params.set("sort", sort);
  const qs = params.toString();
  return request<TopicSummary[]>(`/api/topics${qs ? `?${qs}` : ""}`);
}

export async function fetchKeys(
  topic: string,
  cluster?: string,
  search?: string,
  sortBy?: string,
  sort?: string,
  page = 1,
  limit = 25,
  partitions?: number[],
  minOffset?: number,
): Promise<PaginatedResponse<KeySummary>> {
  const params = new URLSearchParams({ page: String(page), limit: String(limit) });
  if (cluster) params.set("cluster", cluster);
  if (search) params.set("search", search);
  if (sortBy) params.set("sort_by", sortBy);
  if (sort) params.set("sort", sort);
  if (partitions && partitions.length > 0) params.set("partition", partitions.join(","));
  if (minOffset !== undefined && minOffset > 0) params.set("offset", String(minOffset));
  return request<PaginatedResponse<KeySummary>>(
    `/api/topics/${encodeURIComponent(topic)}/keys?${params}`
  );
}

export interface TopicDetail {
  name: string;
  cluster: string;
  partitions: number;
  compacted: boolean;
  message_format: string;
  consumed_messages: number;
  unique_keys: number;
  progress: Progress;
}

export async function fetchTopicDetail(topic: string, cluster?: string): Promise<TopicDetail> {
  const params = cluster ? `?cluster=${encodeURIComponent(cluster)}` : "";
  return request<TopicDetail>(`/api/topics/${encodeURIComponent(topic)}${params}`);
}

export async function reconsumeTopics(topic: string, cluster?: string): Promise<{ deleted: number; topic: string }> {
  const params = cluster ? `?cluster=${encodeURIComponent(cluster)}` : "";
  const res = await fetch(`${API_BASE}/api/topics/${encodeURIComponent(topic)}/reconsume${params}`, { method: "POST" });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

export async function fetchKeyHistory(
  topic: string,
  key: string,
  cluster?: string,
  page = 1,
  limit = 20
): Promise<PaginatedResponse<Message>> {
  const params = new URLSearchParams({ key, page: String(page), limit: String(limit) });
  if (cluster) params.set("cluster", cluster);
  return request<PaginatedResponse<Message>>(
    `/api/topics/${encodeURIComponent(topic)}/history?${params}`
  );
}

export interface OrphanedCluster {
  name: string;
  topic_count: number;
  message_count: number;
  key_count: number;
}

export async function fetchOrphanedClusters(): Promise<OrphanedCluster[]> {
  return request<OrphanedCluster[]>("/api/settings/orphaned-clusters");
}

export async function deleteClusterData(cluster: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/settings/clusters/${encodeURIComponent(cluster)}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
}

export interface SettingsInfo {
  version: string;
  started_at: string;
  uptime: string;
  clusters: Array<{
    name: string;
    bootstrap_servers: string;
    schema_registry: boolean;
    properties?: Record<string, string>;
  }>;
  cluster_status: Array<{ name: string; status: string; paused: boolean }>;
  stats: {
    total_topics: number;
    total_messages: number;
    total_keys: number;
  };
  config: {
    server_port: number;
    db_host: string;
    db_name: string;
    compacted_only: boolean;
  };
}

export async function fetchSettingsInfo(): Promise<SettingsInfo> {
  return request<SettingsInfo>("/api/settings/info");
}

export interface ConsumerGroupInfo {
  group_id: string;
  member_count: number;
  state: string;
  active_on_topic: number;
  total_lag: number;
}

export interface PartitionLag {
  partition: number;
  committed_offset: number;
  end_offset: number;
  lag: number;
  consumer_id: string;
  client_id: string;
  host: string;
}

export interface ConsumerGroupLagDetail {
  group_id: string;
  state: string;
  active_members: number;
  partitions: PartitionLag[];
  total_lag: number;
}

export async function fetchTopicConsumerGroups(topic: string, cluster?: string): Promise<ConsumerGroupInfo[]> {
  const params = cluster ? `?cluster=${encodeURIComponent(cluster)}` : "";
  return request<ConsumerGroupInfo[]>(`/api/topics/${encodeURIComponent(topic)}/consumer-groups${params}`);
}

export async function fetchConsumerGroupLag(topic: string, group: string, cluster?: string): Promise<ConsumerGroupLagDetail> {
  const params = cluster ? `?cluster=${encodeURIComponent(cluster)}` : "";
  return request<ConsumerGroupLagDetail>(`/api/topics/${encodeURIComponent(topic)}/consumer-groups/${encodeURIComponent(group)}${params}`);
}

export async function resetConsumerGroupOffsets(
  topic: string,
  group: string,
  target: "earliest" | "latest" | "specific",
  cluster?: string,
  offsets?: Array<{ partition: number; offset: number }>
): Promise<void> {
  const params = new URLSearchParams();
  if (cluster) params.set("cluster", cluster);
  params.set("target", target);
  const options: RequestInit = { method: "POST" };
  if (target === "specific" && offsets) {
    options.headers = { "Content-Type": "application/json" };
    options.body = JSON.stringify({ offsets });
  }
  const res = await fetch(`${API_BASE}/api/topics/${encodeURIComponent(topic)}/consumer-groups/${encodeURIComponent(group)}/reset-offsets?${params}`, options);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
}
