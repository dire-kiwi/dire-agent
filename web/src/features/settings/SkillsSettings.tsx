import type { GlobalSettings } from "../../lib/protocol";
import { Field, listText, parseList, SettingsSection } from "./SettingsFields";

export function SkillsSettings(props: {
  value: GlobalSettings["skills"];
  onChange: (value: GlobalSettings["skills"]) => void;
}) {
  return (
    <SettingsSection
      id="skills"
      eyebrow="PROGRESSIVE DISCLOSURE"
      title="Agent skills"
      description="Discover Agent Skills-compatible SKILL.md files from global and project roots."
    >
      <div className="settings-grid two">
        <Field label="Skill roots" hint="Absolute paths, one per line." wide>
          <textarea
            rows={5}
            value={listText(props.value.roots)}
            onChange={(event) => props.onChange({ ...props.value, roots: parseList(event.target.value) })}
          />
        </Field>
        <Field label="Trust policy">
          <select
            value={props.value.trust}
            onChange={(event) => props.onChange({ ...props.value, trust: event.target.value as GlobalSettings["skills"]["trust"] })}
          >
            <option value="prompt">Prompt before use</option>
            <option value="trusted">Trusted</option>
            <option value="denied">Denied</option>
          </select>
        </Field>
        <Field label="Disabled skills" hint="Canonical skill file or directory paths." wide>
          <textarea
            rows={4}
            value={listText(props.value.disabled)}
            onChange={(event) => props.onChange({ ...props.value, disabled: parseList(event.target.value) })}
            placeholder="/path/to/skill/SKILL.md"
          />
        </Field>
      </div>
      <div className="info-strip">
        <strong>{props.value.roots.length} discovery roots</strong>
        <span>{props.value.disabled?.length || 0} disabled paths · trust is {props.value.trust}</span>
      </div>
    </SettingsSection>
  );
}
