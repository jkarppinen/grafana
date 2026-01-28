import { css } from '@emotion/css';
import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom-v5-compat';

import { GrafanaTheme2, PageLayoutType } from '@grafana/data';
import { selectors as e2eSelectors } from '@grafana/e2e-selectors';
import { setPublicDashboardVariables } from '@grafana/runtime';
import { SceneComponentProps, UrlSyncContextProvider, sceneGraph } from '@grafana/scenes';
import { Alert, Box, Icon, Stack, useStyles2 } from '@grafana/ui';
import { Page } from 'app/core/components/Page/Page';
import PageLoader from 'app/core/components/PageLoader/PageLoader';
import { GrafanaRouteComponentProps } from 'app/core/navigation/types';
import { DashboardBrandingFooter } from 'app/features/dashboard/components/PublicDashboard/DashboardBrandingFooter';
import { useGetPublicDashboardConfig } from 'app/features/dashboard/components/PublicDashboard/usePublicDashboardConfig';
import { PublicDashboardNotAvailable } from 'app/features/dashboard/components/PublicDashboardNotAvailable/PublicDashboardNotAvailable';
import {
  PublicDashboardPageRouteParams,
  PublicDashboardPageRouteSearchParams,
} from 'app/features/dashboard/containers/types';
import { AppNotificationSeverity } from 'app/types/appNotifications';
import { DashboardRoutes } from 'app/types/dashboard';

import { DashboardScene } from '../scene/DashboardScene';

import { getDashboardScenePageStateManager, LoadError } from './DashboardScenePageStateManager';
import { getPublicDashboardTemplateService } from './PublicDashboardTemplateService';
import { PublicDashboardVariable, PublicDashboardVariableRenderer } from './PublicDashboardVariableRenderer';

const selectors = e2eSelectors.pages.PublicDashboardScene;

export type Props = Omit<
  GrafanaRouteComponentProps<PublicDashboardPageRouteParams, PublicDashboardPageRouteSearchParams>,
  'match' | 'history'
>;

export function PublicDashboardScenePage({ route }: Props) {
  const { accessToken = '' } = useParams();
  const stateManager = getDashboardScenePageStateManager();
  const styles = useStyles2(getStyles);
  const { dashboard, isLoading, loadError } = stateManager.useState();

  useEffect(() => {
    stateManager.loadDashboard({ uid: accessToken, route: DashboardRoutes.Public });

    return () => {
      stateManager.clearState();
    };
  }, [stateManager, accessToken, route.routeName]);

  if (loadError) {
    return <PublicDashboardScenePageError error={loadError} />;
  }

  if (!dashboard) {
    return (
      <Page layout={PageLayoutType.Custom} className={styles.loadingPage} data-testid={selectors.loadingPage}>
        {isLoading && <PageLoader />}
      </Page>
    );
  }

  // if no time picker render without url sync
  if (dashboard.state.controls?.state.hideTimeControls) {
    return <PublicDashboardSceneRenderer model={dashboard} />;
  }

  return (
    <UrlSyncContextProvider scene={dashboard}>
      <PublicDashboardSceneRenderer model={dashboard} />
    </UrlSyncContextProvider>
  );
}

function PublicDashboardSceneRenderer({ model }: SceneComponentProps<DashboardScene>) {
  const [isActive, setIsActive] = useState(false);
  const [variableValues, setVariableValues] = useState<Record<string, string | string[]>>({});
  const { controls, title, body } = model.useState();
  const { timePicker, refreshPicker, hideTimeControls } = controls!.useState();
  const styles = useStyles2(getStyles);
  const conf = useGetPublicDashboardConfig();

  // Extract variables from dashboard data
  const variables = extractVariablesFromDashboard(model);

  // Initialize variable values from URL parameters and scene state on mount
  useEffect(() => {
    const initialValues: Record<string, string | string[]> = {};

    // First, read variables from URL parameters (var-xxx=value)
    const urlParams = new URLSearchParams(window.location.search);
    urlParams.forEach((value, key) => {
      if (key.startsWith('var-') && value) {
        const varName = key.substring(4); // Remove 'var-' prefix
        initialValues[varName] = value;
      }
    });

    // Then, fill in any missing values from scene variable state
    for (const variable of variables) {
      if (!initialValues[variable.name] && variable.current?.value) {
        initialValues[variable.name] = variable.current.value;
      }
    }

    // Always set variables if we have any (from URL or scene)
    if (Object.keys(initialValues).length > 0) {
      console.log('PublicDashboard: Setting variables', initialValues);
      setVariableValues(initialValues);
      // Sync to the query handler
      setPublicDashboardVariables(initialValues);
      getPublicDashboardTemplateService(initialValues);
    }
    // Only run on mount when variables first become available
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [variables.length]);

  const handleVariableChange = (name: string, value: string | string[]) => {
    const newValues = { ...variableValues, [name]: value };
    setVariableValues(newValues);

    // Update both the template service and the query handler
    getPublicDashboardTemplateService(newValues);
    setPublicDashboardVariables(newValues);

    // Trigger dashboard refresh to apply new variable values
    if (refreshPicker) {
      refreshPicker.onRefresh();
    }
  };

  useEffect(() => {
    return refreshPicker.activate();
  }, [refreshPicker]);

  useEffect(() => {
    setIsActive(true);
    return model.activate();
  }, [model]);

  if (!isActive) {
    return null;
  }

  return (
    <Page layout={PageLayoutType.Custom} className={styles.page} data-testid={selectors.page}>
      <div className={styles.controls}>
        <Stack alignItems="center">
          {!conf.headerLogoHide && (
            <div className={styles.iconTitle}>
              <Icon name="grafana" size="lg" aria-hidden />
            </div>
          )}
          <span className={styles.title}>{title}</span>
        </Stack>
        {!hideTimeControls && (
          <Stack>
            <timePicker.Component model={timePicker} />
            <refreshPicker.Component model={refreshPicker} />
          </Stack>
        )}
      </div>
      <PublicDashboardVariableRenderer
        variables={variables}
        onVariableChange={handleVariableChange}
        values={variableValues}
      />
      <div className={styles.body}>
        <body.Component model={body} />
      </div>
      <DashboardBrandingFooter />
    </Page>
  );
}

// Helper function to extract template variables from the scene's variable set
function extractVariablesFromDashboard(model: DashboardScene): PublicDashboardVariable[] {
  // Get variables from the scene using sceneGraph
  const variableSet = sceneGraph.getVariables(model);
  if (!variableSet || !variableSet.state.variables) {
    return [];
  }

  // Map scene variables to our PublicDashboardVariable format
  // Access properties using optional chaining since different variable types have different state properties
  return variableSet.state.variables
    .filter((v) => v.state.name && !v.state.name.startsWith('__'))
    .map((v) => {
      const state = v.state;
      // Access optional properties that may exist on different variable types
      const label = 'label' in state ? (state.label ?? undefined) : undefined;
      const multi = 'multi' in state ? Boolean(state.multi) : false;
      const options =
        'options' in state && Array.isArray(state.options)
          ? state.options.map((opt: { text?: unknown; value?: unknown; selected?: boolean }) => ({
              text: String(opt.text ?? ''),
              value: String(opt.value ?? ''),
              selected: opt.selected,
            }))
          : [];
      // Scene variables store value directly in state.value, not in a current object
      let currentValue: string | string[] | undefined;
      if ('value' in state && state.value !== undefined) {
        const stateValue = state.value;
        if (typeof stateValue === 'string' || Array.isArray(stateValue)) {
          currentValue = stateValue;
        }
      }

      return {
        name: state.name,
        label: typeof label === 'string' ? label : undefined,
        hide: state.hide,
        type: state.type,
        multi,
        options,
        current: currentValue !== undefined ? { value: String(currentValue) } : undefined,
      };
    });
}

function getStyles(theme: GrafanaTheme2) {
  return {
    loadingPage: css({
      justifyContent: 'center',
    }),
    page: css({
      padding: theme.spacing(0, 2),
    }),
    controls: css({
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      position: 'sticky',
      top: 0,
      zIndex: theme.zIndex.navbarFixed,
      background: theme.colors.background.canvas,
      padding: theme.spacing(2, 0),
      [theme.breakpoints.down('sm')]: {
        flexDirection: 'column',
        gap: theme.spacing(1),
        alignItems: 'stretch',
      },
    }),
    iconTitle: css({
      display: 'none',
      [theme.breakpoints.up('sm')]: {
        display: 'flex',
        alignItems: 'center',
      },
    }),
    title: css({
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap',
      display: 'flex',
      fontSize: theme.typography.h4.fontSize,
      margin: 0,
    }),
    body: css({
      label: 'body',
      display: 'flex',
      flex: 1,
      flexDirection: 'column',
      overflowY: 'auto',
    }),
  };
}

function PublicDashboardScenePageError({ error }: { error: LoadError }) {
  const styles = useStyles2(getStyles);
  const statusCode = error.status;
  const messageId = error.messageId;
  const message = error.message;

  const isPublicDashboardPaused = statusCode === 403 && messageId === 'publicdashboards.notEnabled';
  const isPublicDashboardNotFound = statusCode === 404 && messageId === 'publicdashboards.notFound';
  const isDashboardNotFound = statusCode === 404 && messageId === 'publicdashboards.dashboardNotFound';

  const publicDashboardEnabled = isPublicDashboardNotFound ? undefined : !isPublicDashboardPaused;
  const dashboardNotFound = isPublicDashboardNotFound || isDashboardNotFound;

  if (publicDashboardEnabled === false) {
    return <PublicDashboardNotAvailable paused />;
  }

  if (dashboardNotFound) {
    return <PublicDashboardNotAvailable />;
  }

  return (
    <Page layout={PageLayoutType.Custom} className={styles.loadingPage} data-testid={selectors.loadingPage}>
      <Box paddingY={4} display="flex" direction="column" alignItems="center">
        <Alert severity={AppNotificationSeverity.Error} title={message}>
          {message}
        </Alert>
      </Box>
    </Page>
  );
}
