import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSelector } from 'react-redux';
import { API } from 'utils/api';
import { showError } from 'utils/common';
import { defaults, queryParams, normalizeUsage, emptyUsage, requestParams, exportRequests } from './analyticsData';

export default function useDashboardAnalytics(period, setPeriod, metric, setMetric) {
  const user = useSelector((s) => s.account.user);
  const [options, setOptions] = useState(defaults),
    [rank, setRank] = useState('cost'),
    [dialog, setDialog] = useState(null),
    [drawer, setDrawer] = useState(null),
    [page, setPage] = useState(0),
    [detailTab, setDetailTab] = useState('daily'),
    [sort, setSort] = useState({ field: 'cost', direction: -1 }),
    [saved, setSaved] = useState([]),
    [epoch, setEpoch] = useState(0),
    [usage, setUsage] = useState(emptyUsage),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(''),
    [updated, setUpdated] = useState(null),
    [exporting, setExporting] = useState(false);
  const savedKey = 'dashboard-views-v1-' + user?.id;
  useEffect(() => {
    try {
      const list = JSON.parse(localStorage.getItem(savedKey) || '[]');
      setSaved(
        Array.isArray(list)
          ? list.filter((v) => v?.name && ['24h', '7', '30', '90', 'custom'].includes(String(v.period)) && v.options).slice(0, 30)
          : []
      );
    } catch {
      setSaved([]);
    }
  }, [savedKey]);
  const saveViews = (list) => {
    try {
      localStorage.setItem(savedKey, JSON.stringify(list));
      setSaved(list);
    } catch {
      showError('无法保存视图，请检查浏览器存储空间');
    }
  };
  const params = useMemo(() => queryParams(period, options), [period, options]);
  const signature = JSON.stringify(params);
  const latest = useRef(0);
  const previousSignature = useRef(null);
  useEffect(() => {
    setDrawer(null);
    setPage(0);
  }, [signature]);
  useEffect(() => {
    const request = ++latest.current,
      abort = new AbortController();
    setLoading(true);
    setError('');
    if (previousSignature.current !== signature) {
      setUsage((old) => ({ ...emptyUsage(), filters: old.filters }));
      setUpdated(null);
    }
    previousSignature.current = signature;
    API.get('/api/user/dashboard/usage', { params: JSON.parse(signature), skipErrorToast: true, signal: abort.signal })
      .then((res) => {
        if (request !== latest.current) return;
        if (!res.data.success) throw new Error(res.data.message);
        setUsage(normalizeUsage(res.data.data, { zone: JSON.parse(signature).timezone }));
        setUpdated(new Date(res.data.data.generated_at * 1000));
      })
      .catch((e) => {
        if (!abort.signal.aborted && request === latest.current) {
          setError(e.response?.data?.message || e.message || '获取用量失败');
          setUsage((old) => ({ ...emptyUsage(), filters: old.filters }));
          setUpdated(null);
        }
      })
      .finally(() => {
        if (!abort.signal.aborted && request === latest.current) setLoading(false);
      });
    return () => abort.abort();
  }, [signature, epoch]);
  const refresh = useCallback(() => setEpoch((v) => v + 1), []);
  useEffect(() => {
    if (!options.auto) return;
    const id = setInterval(() => {
      if (!document.hidden) refresh();
    }, 30000);
    return () => clearInterval(id);
  }, [options.auto, refresh]);
  const update = (key, value) => setOptions((o) => ({ ...o, [key]: value }));
  const openLogs = ({ error: errorsOnly, ...filter } = {}) => {
    if (loading || error) return;
    setDrawer({ type: 'logs', filter: { ...usage.range, ...filter }, page: 0, search: '', status: errorsOnly ? 'error' : 'all' });
  };
  const p = {
    period,
    setPeriod,
    metric,
    setMetric,
    options,
    setOptions,
    update,
    usage,
    rank,
    setRank,
    dialog,
    setDialog,
    drawer,
    setDrawer,
    openLogs,
    page,
    setPage,
    detailTab,
    setDetailTab,
    sort,
    setSort,
    saved,
    saveViews,
    loading,
    error,
    updated,
    refresh,
    exporting,
    balance: user?.quota
  };
  p.exportRequests = async (filter = {}) => {
    if (exporting) return;
    setExporting(true);
    try {
      await exportRequests(requestParams(p, filter));
    } finally {
      setExporting(false);
    }
  };
  return p;
}

export function useDashboardRequests(params, enabled = true) {
  const [result, setResult] = useState({ items: [], total: 0, loading: false, error: '' });
  const signature = JSON.stringify(params);
  useEffect(() => {
    if (!enabled) return;
    const abort = new AbortController();
    setResult({ items: [], total: 0, loading: true, error: '' });
    const timer = setTimeout(() => {
      API.get('/api/user/dashboard/requests', { params: JSON.parse(signature), skipErrorToast: true, signal: abort.signal })
        .then((res) => {
          if (!res.data.success) throw new Error(res.data.message);
          if (!abort.signal.aborted) setResult({ ...res.data.data, loading: false, error: '' });
        })
        .catch((e) => {
          if (!abort.signal.aborted) setResult({ items: [], total: 0, loading: false, error: e.response?.data?.message || e.message });
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      abort.abort();
    };
  }, [signature, enabled]);
  return result;
}
