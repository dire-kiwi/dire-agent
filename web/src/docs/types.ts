export interface TestStep {
  action: string;
  expected: string;
}

export interface FeatureDoc {
  slug: string;
  title: string;
  group: "Core" | "Agent controls" | "Capabilities" | "Operations";
  summary: string;
  prerequisites: string[];
  steps: TestStep[];
  notes?: string[];
}
