import { agentFeatures } from "./agentFeatures";
import { capabilityFeatures } from "./capabilityFeatures";
import { coreFeatures } from "./coreFeatures";
import type { FeatureDoc } from "./types";

export const featureDocs: FeatureDoc[] = [
  ...coreFeatures,
  ...agentFeatures,
  ...capabilityFeatures,
];

export function featurePath(feature: FeatureDoc): string {
  return `/docs/${feature.slug}`;
}

export function currentFeature(pathname: string): FeatureDoc | undefined {
  const slug = pathname.replace(/^\/docs\/?/, "").replace(/\/$/, "");
  return featureDocs.find((feature) => feature.slug === slug);
}
