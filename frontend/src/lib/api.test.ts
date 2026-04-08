import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import {
  fetchClusters,
  fetchTopics,
  fetchTopicDetail,
  fetchKeys,
  fetchKeyHistory,
  fetchTopicConsumerGroups,
  fetchConsumerGroupLag,
  resetConsumerGroupOffsets,
  deleteClusterData,
  pauseCluster,
  resumeCluster,
  reconsumeTopics,
  fetchOrphanedClusters,
} from "./api";

function mockResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    statusText: status === 200 ? "OK" : "Internal Server Error",
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("fetchClusters", () => {
  test("builds correct URL and returns parsed JSON", async () => {
    const mockFetch = vi.mocked(fetch);
    const data = [{ name: "prod", status: "connected", paused: false }];
    mockFetch.mockResolvedValueOnce(mockResponse(data));

    const result = await fetchClusters();

    expect(mockFetch).toHaveBeenCalledWith("/api/clusters");
    expect(result).toEqual(data);
  });
});

describe("fetchTopics", () => {
  test("builds URL with cluster param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchTopics("my-cluster");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics?cluster=my-cluster");
  });

  test("builds URL without params when none given", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchTopics();

    expect(mockFetch).toHaveBeenCalledWith("/api/topics");
  });

  test("builds URL with sort param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchTopics("cluster1", "name");

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("cluster=cluster1");
    expect(url).toContain("sort=name");
  });
});

describe("fetchTopicDetail", () => {
  test("builds URL with cluster query param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ name: "my-topic" }));

    await fetchTopicDetail("my-topic", "prod");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my-topic?cluster=prod");
  });

  test("builds URL without cluster when not provided", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ name: "my-topic" }));

    await fetchTopicDetail("my-topic");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my-topic");
  });

  test("encodes topic name in URL", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ name: "my topic" }));

    await fetchTopicDetail("my topic");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my%20topic");
  });
});

describe("fetchKeys", () => {
  test("builds URL with all params", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ data: [], total: 0 }));

    await fetchKeys("topic1", "cluster1", "search-term", "last_updated", "desc", 2, 50, [0, 1], 100);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/api/topics/topic1/keys?");
    expect(url).toContain("page=2");
    expect(url).toContain("limit=50");
    expect(url).toContain("cluster=cluster1");
    expect(url).toContain("search=search-term");
    expect(url).toContain("sort_by=last_updated");
    expect(url).toContain("sort=desc");
    expect(url).toContain("partition=0%2C1");
    expect(url).toContain("offset=100");
  });

  test("builds URL with only required params", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ data: [], total: 0 }));

    await fetchKeys("topic1");

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/api/topics/topic1/keys?");
    expect(url).toContain("page=1");
    expect(url).toContain("limit=25");
    expect(url).not.toContain("cluster=");
    expect(url).not.toContain("search=");
  });
});

describe("fetchKeyHistory", () => {
  test("builds URL with topic and key path params and cluster/page/limit query", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ data: [], total: 0 }));

    await fetchKeyHistory("my-topic", "my-key", "prod", 3, 10);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/api/topics/my-topic/history?");
    expect(url).toContain("key=my-key");
    expect(url).toContain("cluster=prod");
    expect(url).toContain("page=3");
    expect(url).toContain("limit=10");
  });

  test("uses default page and limit", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ data: [], total: 0 }));

    await fetchKeyHistory("t", "k");

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("page=1");
    expect(url).toContain("limit=20");
  });
});

describe("fetchTopicConsumerGroups", () => {
  test("builds URL with cluster param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchTopicConsumerGroups("my-topic", "prod");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my-topic/consumer-groups?cluster=prod");
  });

  test("builds URL without cluster param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchTopicConsumerGroups("my-topic");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my-topic/consumer-groups");
  });
});

describe("fetchConsumerGroupLag", () => {
  test("builds URL with group path param and cluster", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ group_id: "g1", total_lag: 0 }));

    await fetchConsumerGroupLag("my-topic", "my-group", "prod");

    expect(mockFetch).toHaveBeenCalledWith("/api/topics/my-topic/consumer-groups/my-group?cluster=prod");
  });
});

describe("resetConsumerGroupOffsets", () => {
  test("POST with target=earliest and no body", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse(null));

    await resetConsumerGroupOffsets("t", "g", "earliest", "prod");

    const [url, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/topics/t/consumer-groups/g/reset-offsets?");
    expect(url).toContain("target=earliest");
    expect(url).toContain("cluster=prod");
    expect(options.method).toBe("POST");
    expect(options.body).toBeUndefined();
  });

  test("POST with target=specific sends JSON body", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse(null));

    const offsets = [{ partition: 0, offset: 100 }];
    await resetConsumerGroupOffsets("t", "g", "specific", "prod", offsets);

    const [, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(options.method).toBe("POST");
    expect(options.headers).toEqual({ "Content-Type": "application/json" });
    expect(JSON.parse(options.body as string)).toEqual({ offsets });
  });

  test("throws on error response with body.error", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "group is active" }), { status: 400, statusText: "Bad Request" })
    );

    await expect(resetConsumerGroupOffsets("t", "g", "earliest")).rejects.toThrow("group is active");
  });
});

describe("deleteClusterData", () => {
  test("sends DELETE request", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse(null));

    await deleteClusterData("old-cluster");

    const [url, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/settings/clusters/old-cluster");
    expect(options.method).toBe("DELETE");
  });

  test("throws on error response", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(new Response("", { status: 500, statusText: "Internal Server Error" }));

    await expect(deleteClusterData("c")).rejects.toThrow("API error: 500");
  });
});

describe("pauseCluster", () => {
  test("sends POST to pause endpoint", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse(null));

    await pauseCluster("prod");

    const [url, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/clusters/prod/pause");
    expect(options.method).toBe("POST");
  });
});

describe("resumeCluster", () => {
  test("sends POST to resume endpoint", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse(null));

    await resumeCluster("prod");

    const [url, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/clusters/prod/resume");
    expect(options.method).toBe("POST");
  });
});

describe("reconsumeTopics", () => {
  test("sends POST to reconsume endpoint with cluster param", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse({ deleted: 10, topic: "t" }));

    const result = await reconsumeTopics("my-topic", "prod");

    const [url, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/topics/my-topic/reconsume?cluster=prod");
    expect(options.method).toBe("POST");
    expect(result).toEqual({ deleted: 10, topic: "t" });
  });
});

describe("fetchOrphanedClusters", () => {
  test("builds correct URL", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(mockResponse([]));

    await fetchOrphanedClusters();

    expect(mockFetch).toHaveBeenCalledWith("/api/settings/orphaned-clusters");
  });
});

describe("request error handling", () => {
  test("throws on non-ok response", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(new Response("", { status: 404, statusText: "Not Found" }));

    await expect(fetchClusters()).rejects.toThrow("API error: 404 Not Found");
  });

  test("throws on 500 response", async () => {
    const mockFetch = vi.mocked(fetch);
    mockFetch.mockResolvedValueOnce(new Response("", { status: 500, statusText: "Internal Server Error" }));

    await expect(fetchTopics()).rejects.toThrow("API error: 500 Internal Server Error");
  });
});
