import { describe, test, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { Sidebar } from "./Sidebar";

vi.mock("@/lib/api", () => ({
  fetchClusters: vi.fn(),
  fetchHealth: vi.fn().mockResolvedValue({ status: "ok", version: "test" }),
}));

import { fetchClusters } from "@/lib/api";

const mockFetchClusters = vi.mocked(fetchClusters);

function renderSidebar(activeCluster: string | null = null) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Sidebar activeCluster={activeCluster} />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("Sidebar", () => {
  test("renders Clusters heading", () => {
    mockFetchClusters.mockResolvedValueOnce([]);
    renderSidebar();
    expect(screen.getByText("Clusters")).toBeInTheDocument();
  });

  test("shows loading skeleton when clusters are loading", () => {
    mockFetchClusters.mockReturnValueOnce(new Promise(() => {}));
    renderSidebar();
    const skeletons = document.querySelectorAll("[class*='animate-pulse']");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  test("renders cluster buttons when data is loaded", async () => {
    mockFetchClusters.mockResolvedValueOnce([
      { name: "prod", status: "connected", paused: false },
      { name: "staging", status: "connecting", paused: false },
    ]);
    renderSidebar();

    expect(await screen.findByText("prod")).toBeInTheDocument();
    expect(screen.getByText("staging")).toBeInTheDocument();
  });

  test("active cluster has different styling", async () => {
    mockFetchClusters.mockResolvedValueOnce([
      { name: "prod", status: "connected", paused: false },
      { name: "staging", status: "connected", paused: false },
    ]);
    renderSidebar("prod");

    const prodBtn = await screen.findByText("prod");
    expect(prodBtn.closest("button")!.className).toContain("bg-primary");
  });

  test("Settings link renders at the bottom", () => {
    mockFetchClusters.mockResolvedValueOnce([]);
    renderSidebar();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  test("logo links to home", () => {
    mockFetchClusters.mockResolvedValueOnce([]);
    renderSidebar();
    const logoLink = document.querySelector("a[href='/']");
    expect(logoLink).not.toBeNull();
  });
});
