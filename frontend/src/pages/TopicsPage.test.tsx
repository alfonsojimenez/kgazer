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
  fetchSettingsInfo: vi.fn().mockResolvedValue({ config: { compacted_only: true } }),
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
    expect(screen.queryByText("No topics found")).not.toBeInTheDocument();
  });

  test("topic names are links to the keys page", async () => {
    mockFetchTopics.mockResolvedValueOnce([makeTopic({ name: "orders" })]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    const link = await screen.findByRole("link", { name: "orders" });
    expect(link).toHaveAttribute("href", "/clusters/test-cluster/topics/orders/keys");
  });

  test("search is read from the URL", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "orders" }),
      makeTopic({ name: "payments" }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />, { route: "/clusters/test-cluster?q=pay" });

    expect(await screen.findByText("payments")).toBeInTheDocument();
    expect(screen.queryByText("orders")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Search topics/)).toHaveValue("pay");
  });

  test("escape clears the search", async () => {
    const user = userEvent.setup();
    mockFetchTopics.mockResolvedValueOnce([makeTopic({ name: "orders" })]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />, { route: "/clusters/test-cluster?q=zzz" });

    const input = await screen.findByPlaceholderText(/Search topics/);
    await user.click(input);
    await user.keyboard("{Escape}");

    expect(input).toHaveValue("");
    expect(screen.getByText("orders")).toBeInTheDocument();
  });

  test("clear button clears the search", async () => {
    const user = userEvent.setup();
    mockFetchTopics.mockResolvedValueOnce([makeTopic({ name: "orders" })]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />, { route: "/clusters/test-cluster?q=zzz" });

    await user.click(await screen.findByRole("button", { name: "Clear search" }));

    expect(screen.getByPlaceholderText(/Search topics/)).toHaveValue("");
  });

  test("sorts by message count", async () => {
    const user = userEvent.setup();
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "small", message_count: 1 }),
      makeTopic({ name: "big", message_count: 999 }),
      makeTopic({ name: "medium", message_count: 50 }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await screen.findByText("small");
    await user.click(screen.getByText("Messages"));

    const names = screen.getAllByRole("link").map((a) => a.textContent);
    expect(names).toEqual(["big", "medium", "small"]);
  });

  test("shows syncing progress only for topics still consuming", async () => {
    mockFetchTopics.mockResolvedValueOnce([
      makeTopic({ name: "done-topic" }),
      makeTopic({ name: "sync-topic", progress: { total: 1000, consumed: 400, percent: 40, done: false } }),
    ]);
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText(/40% · 600 left/)).toBeInTheDocument();
    expect(screen.getByText(/1 syncing/)).toBeInTheDocument();
    expect(screen.queryByText("Done")).not.toBeInTheDocument();
  });

  test("explains empty state for disconnected cluster", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([
      { name: "test-cluster", status: "disconnected", paused: false },
    ]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("Can't reach test-cluster")).toBeInTheDocument();
  });

  test("explains empty state for paused cluster", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([
      { name: "test-cluster", status: "connected", paused: true },
    ]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText("Consumption is paused for test-cluster")).toBeInTheDocument();
  });

  test("mentions compactedOnly in empty state", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([
      { name: "test-cluster", status: "connected", paused: false },
    ]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByText(/Only compacted topics are shown/)).toBeInTheDocument();
  });

  test("status dot has an accessible label", async () => {
    mockFetchTopics.mockResolvedValueOnce([]);
    mockFetchClusters.mockResolvedValueOnce([
      { name: "test-cluster", status: "connected", paused: false },
    ]);
    renderWithProviders(<TopicsPage />);

    expect(await screen.findByRole("img", { name: "Connected" })).toBeInTheDocument();
  });

  test("virtualizes large topic lists", async () => {
    mockFetchTopics.mockResolvedValueOnce(
      Array.from({ length: 1000 }, (_, i) => makeTopic({ name: `topic-${i}` }))
    );
    mockFetchClusters.mockResolvedValueOnce([]);
    renderWithProviders(<TopicsPage />);

    await screen.findByText(/1000 topics/);
    expect(screen.queryAllByRole("link").length).toBeLessThan(1000);
  });
});
