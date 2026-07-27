import { describe, test, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { TopicsPage } from "./TopicsPage";
import type { TopicSummary, ClusterInfo } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  fetchTopics: vi.fn(),
  fetchClusters: vi.fn(),
}));

import { fetchTopics, fetchClusters } from "@/lib/api";

const mockFetchTopics = vi.mocked(fetchTopics);
const mockFetchClusters = vi.mocked(fetchClusters);

function makeTopic(overrides: Partial<TopicSummary> = {}): TopicSummary {
  return {
    name: "test-topic",
    cluster: "test-cluster",
    partitions: 3,
    compacted: false,
    message_format: "json",
    message_count: 100,
    key_count: 50,
    last_updated: new Date().toISOString(),
    progress: { total: 100, consumed: 100, percent: 100, done: true },
    ...overrides,
  };
}

function renderWithProviders(
  ui: React.ReactElement,
  { route = "/clusters/test-cluster" } = {}
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path="/clusters/:cluster" element={ui} />
          <Route path="/" element={ui} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("TopicsPage", () => {
  test("renders loading skeleton", () => {
    mockFetchTopics.mockReturnValueOnce(new Promise(() => {}));
    mockFetchClusters.mockReturnValueOnce(new Promise(() => {}));
    renderWithProviders(<TopicsPage />);

    const skeletons = document.querySelectorAll("[class*='animate-pulse']");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  test("renders topic list when data is loaded", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "orders" }),
      makeTopic({ name: "payments" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("orders")).toBeInTheDocument();
    expect(screen.getByText("payments")).toBeInTheDocument();
  });

  test("shows cluster name in heading when cluster is set", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([
      { name: "test-cluster", status: "connected", paused: false },
    ] as ClusterInfo[]);
    renderWithProviders(<TopicsPage />);

    await waitFor(() => {
      expect(screen.getByText("test-cluster")).toBeInTheDocument();
    });
  });

  test("search input filters topics", async () => {
    const user = userEvent.setup();
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "orders" }),
      makeTopic({ name: "payments" }),
      makeTopic({ name: "order-events" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await screen.findByText("orders");

    const searchInput = screen.getByPlaceholderText(/Search topics/);
    await user.type(searchInput, "order");

    expect(screen.getByText("orders")).toBeInTheDocument();
    expect(screen.getByText("order-events")).toBeInTheDocument();
    expect(screen.queryByText("payments")).not.toBeInTheDocument();
  });

  test("topic format badges render for avro/json", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "json-topic", message_format: "json" }),
      makeTopic({ name: "avro-topic", message_format: "avro" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("JSON")).toBeInTheDocument();
    expect(screen.getByText("Avro")).toBeInTheDocument();
  });

  test("topic format badge renders for protobuf", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "proto-topic", message_format: "protobuf" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("Protobuf")).toBeInTheDocument();
  });

  test("no badge for 'other' format", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "binary-topic", message_format: "other" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await screen.findByText("binary-topic");
    expect(screen.queryByText("Other")).not.toBeInTheDocument();
    expect(screen.queryByText("Protobuf")).not.toBeInTheDocument();
  });

  test("summary stats show correct totals", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "t1", message_count: 1000, key_count: 200 }),
      makeTopic({ name: "t2", message_count: 500, key_count: 100 }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await waitFor(() => {
      expect(screen.getByText(/1,?500 messages/)).toBeInTheDocument();
      expect(screen.getByText(/300 unique keys/)).toBeInTheDocument();
    });
  });

  test("shows empty state when no topics", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("No topics found")).toBeInTheDocument();
  });

  test("shows no-match state when search has no results", async () => {
    const user = userEvent.setup();
    mockFetchTopics.mockResolvedValueOnce([makeTopic({ name: "orders" })]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await screen.findByText("orders");

    const searchInput = screen.getByPlaceholderText(/Search topics/);
    await user.type(searchInput, "zzzzz");

    expect(screen.getByText("No topics matching your search")).toBeInTheDocument();
  });

  test("shows error state when fetch fails", async () => {
    mockFetchTopics.mockRejectedValueOnce(new Error("Network error"));
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText(/Failed to load topics/)).toBeInTheDocument();
  });
});
