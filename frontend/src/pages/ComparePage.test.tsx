import { describe, test, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { ComparePage } from "./ComparePage";
import { compareUrl } from "@/lib/compare";
import type { Message } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  fetchClusters: vi.fn().mockResolvedValue([{ name: "staging" }, { name: "prod" }]),
  fetchKeyHistory: vi.fn(),
}));

import { fetchKeyHistory } from "@/lib/api";

const mockHistory = vi.mocked(fetchKeyHistory);

function msg(body: unknown, offset: number): Message {
  return { id: offset, key: "k", body, partition: 0, offset, timestamp: "2026-01-01T00:00:00Z" };
}

function renderAt(url: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route path="/compare" element={<ComparePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("compareUrl", () => {
  test("prefills the right side with the left cluster and topic", () => {
    const url = new URL(compareUrl({ cluster: "prod", topic: "t", key: "k" }), "http://x");
    expect(url.pathname).toBe("/compare");
    expect(Object.fromEntries(url.searchParams)).toEqual({ lc: "prod", lt: "t", lk: "k", rc: "prod", rt: "t", rk: "" });
  });
});

describe("ComparePage", () => {
  test("diffs the latest value of both sides", async () => {
    mockHistory.mockImplementation(async (_topic, _key, cluster) => ({
      data: [cluster === "staging" ? msg({ price: 1 }, 3) : msg({ price: 2 }, 7)],
      total: 1,
    }));

    renderAt(compareUrl({ cluster: "staging", topic: "t", key: "k" }, { cluster: "prod", topic: "t", key: "k" }));

    await waitFor(() => expect(screen.getByText(/Hide diff/)).toBeInTheDocument());
    expect(mockHistory).toHaveBeenCalledWith("t", "k", "staging", 1, 1);
    expect(mockHistory).toHaveBeenCalledWith("t", "k", "prod", 1, 1);
    expect(screen.getByText(/^-\s+"price": 1$/)).toBeInTheDocument();
    expect(screen.getByText(/^\+\s+"price": 2$/)).toBeInTheDocument();
  });

  test("reports a missing key", async () => {
    mockHistory.mockImplementation(async (_topic, key) => ({
      data: key === "missing" ? [] : [msg({ a: 1 }, 1)],
      total: 0,
    }));

    renderAt(compareUrl({ cluster: "prod", topic: "t", key: "k" }, { cluster: "prod", topic: "t", key: "missing" }));

    expect(await screen.findByText("Key not found")).toBeInTheDocument();
  });

  test("does not fetch until both sides are complete", () => {
    renderAt(compareUrl({ cluster: "prod", topic: "t", key: "k" }));
    expect(mockHistory).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Compare" })).toBeDisabled();
  });
});
