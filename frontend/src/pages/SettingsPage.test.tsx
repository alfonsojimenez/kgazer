import { describe, test, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { SettingsPage } from "./SettingsPage";

vi.mock("@/lib/api", () => ({
  fetchOrphanedClusters: vi.fn(),
  deleteClusterData: vi.fn(),
  fetchSettingsInfo: vi.fn().mockResolvedValue({
    version: "0.1.0",
    started_at: new Date().toISOString(),
    uptime: "1h0m0s",
    clusters: [],
    cluster_status: [],
    stats: { total_topics: 0, total_messages: 0, total_keys: 0 },
    config: { server_port: 8080, db_host: "localhost", db_name: "kgazer", compacted_only: true },
  }),
}));

import { fetchOrphanedClusters, deleteClusterData } from "@/lib/api";

const mockFetchOrphanedClusters = vi.mocked(fetchOrphanedClusters);
const mockDeleteClusterData = vi.mocked(deleteClusterData);

function renderSettings() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SettingsPage", () => {
  test("renders heading", async () => {
    mockFetchOrphanedClusters.mockResolvedValueOnce([]);
    renderSettings();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  test("renders orphaned clusters section heading", async () => {
    mockFetchOrphanedClusters.mockResolvedValueOnce([]);
    renderSettings();
    expect(screen.getByText("Orphaned Clusters")).toBeInTheDocument();
  });

  test("shows empty state when no orphaned clusters", async () => {
    mockFetchOrphanedClusters.mockResolvedValueOnce([]);
    renderSettings();

    expect(
      await screen.findByText(/No orphaned clusters found/)
    ).toBeInTheDocument();
  });

  test("shows orphaned cluster table when data exists", async () => {
    mockFetchOrphanedClusters.mockResolvedValueOnce([
      { name: "old-cluster", topic_count: 5, message_count: 1000, key_count: 200 },
    ]);
    renderSettings();

    expect(await screen.findByText("old-cluster")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("1,000")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
  });

  test("delete button opens confirmation dialog", async () => {
    const user = userEvent.setup();
    mockFetchOrphanedClusters.mockResolvedValueOnce([
      { name: "old-cluster", topic_count: 5, message_count: 1000, key_count: 200 },
    ]);
    renderSettings();

    const deleteBtn = await screen.findByText("Delete Data");
    await user.click(deleteBtn);

    expect(screen.getByText("Delete cluster data")).toBeInTheDocument();
    expect(screen.getByText(/permanently delete all topics/)).toBeInTheDocument();
  });

  test("confirming delete calls deleteClusterData", async () => {
    const user = userEvent.setup();
    mockFetchOrphanedClusters.mockResolvedValueOnce([
      { name: "old-cluster", topic_count: 5, message_count: 1000, key_count: 200 },
    ]);
    mockDeleteClusterData.mockResolvedValueOnce(undefined);
    renderSettings();

    const deleteBtn = await screen.findByText("Delete Data");
    await user.click(deleteBtn);

    const confirmBtn = screen.getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    expect(mockDeleteClusterData.mock.calls[0][0]).toBe("old-cluster");
  });

  test("shows loading skeleton initially", () => {
    mockFetchOrphanedClusters.mockReturnValueOnce(new Promise(() => {}));
    renderSettings();

    const skeletons = document.querySelectorAll("[class*='animate-pulse']");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  test("shows error state when fetch fails", async () => {
    mockFetchOrphanedClusters.mockRejectedValueOnce(new Error("Server error"));
    renderSettings();

    expect(await screen.findByText(/Failed to load orphaned clusters/)).toBeInTheDocument();
  });
});
