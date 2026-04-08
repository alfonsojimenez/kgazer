import { describe, test, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { JsonViewer } from "./JsonViewer";

describe("JsonViewer", () => {
  test("renders JSON data with syntax highlighting", () => {
    render(<JsonViewer data={{ name: "test", value: 42 }} />);
    const code = document.querySelector("code");
    expect(code).not.toBeNull();
    expect(code!.innerHTML).toContain("name");
    expect(code!.innerHTML).toContain("test");
    expect(code!.innerHTML).toContain("42");
    expect(code!.innerHTML).toContain("span");
  });

  test("renders short JSON fully without expand button", () => {
    render(<JsonViewer data={{ a: 1 }} />);
    expect(screen.queryByText(/Show all/)).not.toBeInTheDocument();
  });

  test("collapses long JSON by default and shows expand button", () => {
    const bigObject: Record<string, number> = {};
    for (let i = 0; i < 30; i++) {
      bigObject[`key${i}`] = i;
    }
    render(<JsonViewer data={bigObject} />);

    expect(screen.getByText(/Show all \d+ lines/)).toBeInTheDocument();
  });

  test("expand/collapse toggle works", async () => {
    const user = userEvent.setup();
    const bigObject: Record<string, number> = {};
    for (let i = 0; i < 30; i++) {
      bigObject[`key${i}`] = i;
    }
    render(<JsonViewer data={bigObject} />);

    const expandBtn = screen.getByText(/Show all \d+ lines/);
    await user.click(expandBtn);
    expect(screen.getByText(/Collapse/)).toBeInTheDocument();

    await user.click(screen.getByText(/Collapse/));
    expect(screen.getByText(/Show all \d+ lines/)).toBeInTheDocument();
  });

  test("copy button renders", () => {
    render(<JsonViewer data={{ a: 1 }} />);
    const copyButton = document.querySelector("button");
    expect(copyButton).not.toBeNull();
  });

  test("forceExpanded prop controls expansion", () => {
    const bigObject: Record<string, number> = {};
    for (let i = 0; i < 30; i++) {
      bigObject[`key${i}`] = i;
    }
    const { rerender } = render(<JsonViewer data={bigObject} forceExpanded={true} />);
    expect(screen.getByText(/Collapse/)).toBeInTheDocument();

    rerender(<JsonViewer data={bigObject} forceExpanded={false} />);
    expect(screen.getByText(/Show all \d+ lines/)).toBeInTheDocument();
  });

  test("handles null data", () => {
    render(<JsonViewer data={null} />);
    const code = document.querySelector("code");
    expect(code).not.toBeNull();
    expect(code!.textContent).toContain("null");
  });

  test("handles string data", () => {
    render(<JsonViewer data="hello world" />);
    const code = document.querySelector("code");
    expect(code).not.toBeNull();
    expect(code!.textContent).toContain("hello world");
  });

  test("handles array data", () => {
    render(<JsonViewer data={[1, 2, 3]} />);
    const code = document.querySelector("code");
    expect(code).not.toBeNull();
    expect(code!.textContent).toContain("1");
    expect(code!.textContent).toContain("2");
    expect(code!.textContent).toContain("3");
  });
});
