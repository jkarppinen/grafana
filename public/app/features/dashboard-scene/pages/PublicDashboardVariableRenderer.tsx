import { css } from '@emotion/css';
import { useCallback, useEffect, useState } from 'react';

import { GrafanaTheme2, VariableHide } from '@grafana/data';
import { fetchPublicDashboardVariableOptions, MetricFindValue } from '@grafana/runtime';
import { Select, useStyles2, InlineField, MultiSelect, AsyncSelect, AsyncMultiSelect } from '@grafana/ui';

/**
 * Template variable definition for public dashboard rendering
 */
export interface PublicDashboardVariable {
  name: string;
  label?: string;
  hide?: VariableHide;
  type?: string;
  multi?: boolean;
  options?: Array<{
    text: string | string[];
    value: string | string[];
    selected?: boolean;
  }>;
  current?: {
    text?: string | string[];
    value?: string | string[];
  };
}

interface PublicDashboardVariableRendererProps {
  variables: PublicDashboardVariable[];
  onVariableChange: (name: string, value: string | string[]) => void;
  values: Record<string, string | string[]>;
}

/**
 * Renders variable selectors for public dashboards
 *
 * This component displays template variable controls that allow users
 * to change variable values in public dashboard contexts.
 */
export function PublicDashboardVariableRenderer({
  variables,
  onVariableChange,
  values,
}: PublicDashboardVariableRendererProps) {
  const styles = useStyles2(getStyles);

  // Filter out hidden variables and system variables
  const visibleVariables = variables.filter(
    (v) => v.name && !v.name.startsWith('__') && v.hide !== VariableHide.hideVariable
  );

  if (visibleVariables.length === 0) {
    return null;
  }

  return (
    <div className={styles.container}>
      {visibleVariables.map((variable) => (
        <VariableControl
          key={variable.name}
          variable={variable}
          value={values[variable.name]}
          onChange={(value) => onVariableChange(variable.name, value)}
          allValues={values}
        />
      ))}
    </div>
  );
}

interface VariableControlProps {
  variable: PublicDashboardVariable;
  value: string | string[] | undefined;
  onChange: (value: string | string[]) => void;
  allValues: Record<string, string | string[]>;
}

function VariableControl({ variable, value, onChange, allValues }: VariableControlProps) {
  const styles = useStyles2(getStyles);
  const [dynamicOptions, setDynamicOptions] = useState<Array<{ label: string; value: string }>>([]);
  const [isLoading, setIsLoading] = useState(false);

  // Check if this is a query variable that needs dynamic options
  const isQueryVariable = variable.type === 'query';

  // Build options from variable definition (static options)
  const staticOptions =
    variable.options?.map((opt) => ({
      label: Array.isArray(opt.text) ? opt.text.join(', ') : String(opt.text),
      value: Array.isArray(opt.value) ? opt.value.join(',') : String(opt.value),
    })) ?? [];

  // Use dynamic options for query variables, static options otherwise
  const options = isQueryVariable && dynamicOptions.length > 0 ? dynamicOptions : staticOptions;

  // Fetch options for query variables
  useEffect(() => {
    if (!isQueryVariable) {
      return;
    }

    const fetchOptions = async () => {
      setIsLoading(true);
      try {
        // Pass other variable values for cascading variables
        const otherVariables: Record<string, unknown> = {};
        for (const [name, val] of Object.entries(allValues)) {
          if (name !== variable.name) {
            otherVariables[name] = val;
          }
        }

        const results = await fetchPublicDashboardVariableOptions(variable.name, otherVariables);
        const newOptions = results.map((opt: MetricFindValue) => ({
          label: opt.text,
          value: opt.value,
        }));
        setDynamicOptions(newOptions);
      } catch (error) {
        console.error('Failed to fetch variable options:', error);
        // Fall back to static options on error
        setDynamicOptions([]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchOptions();
    // Re-fetch when other variable values change (for cascading variables)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [variable.name, variable.type, JSON.stringify(allValues)]);

  // Load options callback for async select (used for search filtering)
  const loadOptions = useCallback(
    async (inputValue: string) => {
      try {
        const otherVariables: Record<string, unknown> = {};
        for (const [name, val] of Object.entries(allValues)) {
          if (name !== variable.name) {
            otherVariables[name] = val;
          }
        }

        const results = await fetchPublicDashboardVariableOptions(variable.name, otherVariables, inputValue);
        return results.map((opt: MetricFindValue) => ({
          label: opt.text,
          value: opt.value,
        }));
      } catch (error) {
        console.error('Failed to fetch variable options:', error);
        return [];
      }
    },
    [variable.name, allValues]
  );

  // Get current value, falling back to the variable's current value or first option
  const currentValue =
    value ??
    (variable.current?.value
      ? Array.isArray(variable.current.value)
        ? variable.current.value
        : variable.current.value
      : options[0]?.value);

  const label = variable.label || variable.name;
  const hideLabel = variable.hide === VariableHide.hideLabel;

  if (variable.multi) {
    const multiValue = Array.isArray(currentValue) ? currentValue : currentValue ? [currentValue] : [];

    // Use AsyncMultiSelect for query variables to support search filtering
    if (isQueryVariable) {
      return (
        <InlineField label={hideLabel ? undefined : label} className={styles.field}>
          <AsyncMultiSelect
            loadOptions={loadOptions}
            defaultOptions={options}
            value={multiValue.map((v) => ({ label: v, value: v }))}
            onChange={(selected) => onChange(selected.map((s) => s.value!))}
            className={styles.select}
            isClearable={false}
            isLoading={isLoading}
          />
        </InlineField>
      );
    }

    return (
      <InlineField label={hideLabel ? undefined : label} className={styles.field}>
        <MultiSelect
          options={options}
          value={multiValue}
          onChange={(selected) => onChange(selected.map((s) => s.value!))}
          className={styles.select}
          isClearable={false}
        />
      </InlineField>
    );
  }

  // Use AsyncSelect for query variables to support search filtering
  if (isQueryVariable) {
    const selectedValue = Array.isArray(currentValue) ? currentValue[0] : currentValue;
    return (
      <InlineField label={hideLabel ? undefined : label} className={styles.field}>
        <AsyncSelect
          loadOptions={loadOptions}
          defaultOptions={options}
          value={selectedValue ? { label: selectedValue, value: selectedValue } : undefined}
          onChange={(selected) => onChange(selected?.value ?? '')}
          className={styles.select}
          isClearable={false}
          isLoading={isLoading}
        />
      </InlineField>
    );
  }

  return (
    <InlineField label={hideLabel ? undefined : label} className={styles.field}>
      <Select
        options={options}
        value={Array.isArray(currentValue) ? currentValue[0] : currentValue}
        onChange={(selected) => onChange(selected.value!)}
        className={styles.select}
        isClearable={false}
      />
    </InlineField>
  );
}

function getStyles(theme: GrafanaTheme2) {
  return {
    container: css({
      display: 'flex',
      flexWrap: 'wrap',
      gap: theme.spacing(1),
      padding: theme.spacing(1, 0),
    }),
    field: css({
      marginBottom: 0,
    }),
    select: css({
      minWidth: 120,
    }),
  };
}
