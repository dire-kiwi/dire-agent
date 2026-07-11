import { describe, expect, it } from "vitest";
import { currentFeature, featureDocs, featurePath } from "./features";

describe("feature documentation", () => {
  it("has one unique route and executable Web UI procedure per feature", () => {
    const slugs = new Set<string>();
    for (const feature of featureDocs) {
      expect(feature.slug).toMatch(/^[a-z0-9-]+$/);
      expect(slugs.has(feature.slug)).toBe(false);
      expect(feature.prerequisites.length).toBeGreaterThan(0);
      expect(feature.steps.length).toBeGreaterThanOrEqual(3);
      for (const step of feature.steps) {
        expect(step.action.length).toBeGreaterThan(12);
        expect(step.expected.length).toBeGreaterThan(12);
      }
      slugs.add(feature.slug);
      expect(currentFeature(featurePath(feature))).toBe(feature);
    }
  });
});
