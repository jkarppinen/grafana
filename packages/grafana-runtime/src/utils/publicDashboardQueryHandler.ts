import { catchError, lastValueFrom, Observable, of, switchMap } from 'rxjs';

import { DataQuery, DataQueryRequest, DataQueryResponse, LoadingState } from '@grafana/data';

import { config } from '../config';
import { getBackendSrv } from '../services/backendSrv';

import { BackendDataSourceResponse, toDataQueryResponse } from './queryResponse';

/**
 * Response type for variable options from the API
 */
export interface MetricFindValue {
  text: string;
  value: string;
}

// Variable storage for public dashboard queries
let publicDashboardVariables: Record<string, unknown> = {};

/**
 * Set the current variable values for public dashboard queries.
 * This should be called from the public dashboard page when variables change.
 */
export function setPublicDashboardVariables(variables: Record<string, unknown>) {
  publicDashboardVariables = { ...variables };
}

/**
 * Get the current variable values for public dashboard queries.
 */
export function getPublicDashboardVariables(): Record<string, unknown> {
  return publicDashboardVariables;
}

export function publicDashboardQueryHandler(request: DataQueryRequest<DataQuery>): Observable<DataQueryResponse> {
  const {
    intervalMs,
    maxDataPoints,
    requestId,
    panelId,
    queryCachingTTL,
    range: { from: fromRange, to: toRange },
  } = request;
  // Return early if no queries exist
  if (!request.targets.length) {
    return of({ data: [] });
  }

  // If panel ID is undefined, this is likely a variable query (not a panel query).
  // For public dashboards, variable queries should return empty data - variables
  // use their pre-configured values from the URL or dashboard configuration.
  if (panelId === undefined || panelId === null) {
    return of({
      data: [],
      state: LoadingState.Done,
    });
  }

  // Get variables from the public dashboard variable store
  // This is populated by the PublicDashboardScenePage when variables change
  let variables: Record<string, unknown> = { ...publicDashboardVariables };

  // If no variables in store, try to read from URL parameters as fallback
  // This handles the timing issue where queries fire before variables are set
  if (Object.keys(variables).length === 0 && typeof window !== 'undefined') {
    console.log('publicDashboardQueryHandler: URL search:', window.location.search);
    const urlParams = new URLSearchParams(window.location.search);
    urlParams.forEach((value, key) => {
      console.log('publicDashboardQueryHandler: URL param:', key, '=', value);
      if (key.startsWith('var-') && value) {
        const varName = key.substring(4); // Remove 'var-' prefix
        variables[varName] = value;
      }
    });
  }

  // Debug: log variables being sent
  console.log('publicDashboardQueryHandler: sending variables', variables, 'panelId', panelId);

  const body = {
    intervalMs,
    maxDataPoints,
    queryCachingTTL,
    timeRange: {
      from: fromRange.valueOf().toString(),
      to: toRange.valueOf().toString(),
      timezone: request.timezone,
    },
    // Include variables for backend processing
    variables,
  };

  return getBackendSrv()
    .fetch<BackendDataSourceResponse>({
      url: `/api/public/dashboards/${config.publicDashboardAccessToken!}/panels/${panelId}/query`,
      method: 'POST',
      data: body,
      requestId,
    })
    .pipe(
      switchMap((raw) => {
        return of(toDataQueryResponse(raw, request.targets));
      }),
      catchError((err) => {
        return of(toDataQueryResponse(err));
      })
    );
}

/**
 * Fetch variable options for a public dashboard variable.
 * This calls the backend API endpoint for variable queries.
 *
 * @param variableName - The name of the variable to fetch options for
 * @param variables - Optional map of other variable values for cascading variables
 * @param searchFilter - Optional search filter to filter results
 * @returns Promise with array of options in {text, value} format
 */
export async function fetchPublicDashboardVariableOptions(
  variableName: string,
  variables?: Record<string, unknown>,
  searchFilter?: string
): Promise<MetricFindValue[]> {
  const accessToken = config.publicDashboardAccessToken;

  if (!accessToken) {
    console.warn('fetchPublicDashboardVariableOptions: No public dashboard access token available');
    return [];
  }

  try {
    const response = await lastValueFrom(
      getBackendSrv().fetch<MetricFindValue[]>({
        url: `/api/public/dashboards/${accessToken}/variables/${encodeURIComponent(variableName)}/query`,
        method: 'POST',
        data: {
          variables: variables ?? {},
          searchFilter: searchFilter ?? '',
        },
      })
    );

    return response.data ?? [];
  } catch (error) {
    console.error('fetchPublicDashboardVariableOptions: Failed to fetch variable options', error);
    return [];
  }
}
