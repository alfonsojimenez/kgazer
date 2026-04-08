import { describe, test, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Breadcrumbs } from "./Breadcrumbs";

function renderBreadcrumbs(items: Array<{ label: string; to?: string }>) {
  return render(
    <MemoryRouter>
      <Breadcrumbs items={items} />
    </MemoryRouter>
  );
}

describe("Breadcrumbs", () => {
  test("renders all breadcrumb labels", () => {
    renderBreadcrumbs([
      { label: "Topics", to: "/" },
      { label: "my-topic", to: "/topics/my-topic" },
      { label: "my-key" },
    ]);

    expect(screen.getByText("Topics")).toBeInTheDocument();
    expect(screen.getByText("my-topic")).toBeInTheDocument();
    expect(screen.getByText("my-key")).toBeInTheDocument();
  });

  test("renders links for items with 'to' property", () => {
    renderBreadcrumbs([
      { label: "Topics", to: "/topics" },
      { label: "Current" },
    ]);

    const link = screen.getByText("Topics");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("/topics");
  });

  test("renders plain text for items without 'to' property", () => {
    renderBreadcrumbs([
      { label: "Topics", to: "/" },
      { label: "Current" },
    ]);

    const current = screen.getByText("Current");
    expect(current.tagName).toBe("SPAN");
  });

  test("renders separator chevrons between items", () => {
    renderBreadcrumbs([
      { label: "A", to: "/" },
      { label: "B", to: "/b" },
      { label: "C" },
    ]);

    const svgs = document.querySelectorAll("svg");
    expect(svgs.length).toBe(2);
  });
});
