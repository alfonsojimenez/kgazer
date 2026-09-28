import { describe, test, expect } from "vitest";
import { parseSearch, buildSearch, filtersToJSON } from "./search-parser";

describe("parseSearch", () => {
  test("plain text without filters", () => {
    const result = parseSearch("Hubstaff");
    expect(result.text).toBe("Hubstaff");
    expect(result.filters).toEqual({});
  });

  test("empty input", () => {
    const result = parseSearch("");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({});
  });

  test("flat field:value filter", () => {
    const result = parseSearch("globalProductName:Hubstaff");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({ globalProductName: "Hubstaff" });
  });

  test("quoted value with spaces", () => {
    const result = parseSearch('status:"in progress"');
    expect(result.text).toBe("");
    expect(result.filters).toEqual({ status: "in progress" });
  });

  test("text and filter combined", () => {
    const result = parseSearch("site:Capterra some text");
    expect(result.text).toBe("some text");
    expect(result.filters).toEqual({ site: "Capterra" });
  });

  test("value containing colons", () => {
    const result = parseSearch("type:ca:click");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({ type: "ca:click" });
  });

  test("multiple flat filters", () => {
    const result = parseSearch("ownerId:2101498 periodType:month");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({
      ownerId: "2101498",
      periodType: "month",
    });
  });

  test("dotted nested key path", () => {
    const result = parseSearch("spendBreakdown.type:ca:click");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({ "spendBreakdown.type": "ca:click" });
  });

  test("array notation key path", () => {
    const result = parseSearch("spendBreakdown[].type:ca:click");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({
      "spendBreakdown[].type": "ca:click",
    });
  });

  test("nested dotted path with array at intermediate level", () => {
    const result = parseSearch("data.items[].name:Widget");
    expect(result.text).toBe("");
    expect(result.filters).toEqual({
      "data.items[].name": "Widget",
    });
  });

  test("mixed flat and nested filters with text", () => {
    const result = parseSearch("ownerId:2101498 spendBreakdown[].type:ca:click some text");
    expect(result.text).toBe("some text");
    expect(result.filters).toEqual({
      ownerId: "2101498",
      "spendBreakdown[].type": "ca:click",
    });
  });

  test("quoted value with nested path", () => {
    const result = parseSearch('spendBreakdown[].type:"ca:click"');
    expect(result.text).toBe("");
    expect(result.filters).toEqual({
      "spendBreakdown[].type": "ca:click",
    });
  });
});

describe("buildSearch", () => {
  test("flat filter round-trip", () => {
    const input = "ownerId:2101498";
    const parsed = parseSearch(input);
    expect(buildSearch(parsed)).toBe(input);
  });

  test("nested array filter round-trip", () => {
    const input = "spendBreakdown[].type:ca:click";
    const parsed = parseSearch(input);
    expect(buildSearch(parsed)).toBe(input);
  });

  test("filter with spaces gets quoted", () => {
    const parsed = {
      text: "",
      filters: { status: "in progress" },
    };
    expect(buildSearch(parsed)).toBe('status:"in progress"');
  });

  test("removing a filter rebuilds correctly", () => {
    const parsed = parseSearch("ownerId:2101498 spendBreakdown[].type:ca:click");
    delete parsed.filters["spendBreakdown[].type"];
    expect(buildSearch(parsed)).toBe("ownerId:2101498");
  });
});

describe("filtersToJSON", () => {
  test("flat filter produces flat JSON", () => {
    const result = filtersToJSON({ ownerId: "2101498" });
    expect(JSON.parse(result)).toEqual({ ownerId: "2101498" });
  });

  test("dotted path produces nested object", () => {
    const result = filtersToJSON({ "spendBreakdown.type": "ca:click" });
    expect(JSON.parse(result)).toEqual({
      spendBreakdown: { type: "ca:click" },
    });
  });

  test("array notation produces array with object element", () => {
    const result = filtersToJSON({ "spendBreakdown[].type": "ca:click" });
    expect(JSON.parse(result)).toEqual({
      spendBreakdown: [{ type: "ca:click" }],
    });
  });

  test("multiple flat and nested filters merge", () => {
    const result = filtersToJSON({
      ownerId: "2101498",
      "spendBreakdown[].type": "ca:click",
    });
    expect(JSON.parse(result)).toEqual({
      ownerId: "2101498",
      spendBreakdown: [{ type: "ca:click" }],
    });
  });

  test("two array sub-fields create separate array elements", () => {
    const result = filtersToJSON({
      "spendBreakdown[].type": "ca:click",
      "spendBreakdown[].amount": "11953.5",
    });
    expect(JSON.parse(result)).toEqual({
      spendBreakdown: [{ type: "ca:click" }, { amount: "11953.5" }],
    });
  });

  test("deeply nested array path", () => {
    const result = filtersToJSON({ "data.items[].name": "Widget" });
    expect(JSON.parse(result)).toEqual({
      data: { items: [{ name: "Widget" }] },
    });
  });

  test("empty filters produces empty object", () => {
    const result = filtersToJSON({});
    expect(JSON.parse(result)).toEqual({});
  });
});
