'use client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import {
  checkDNSZoneDelegation,
  deleteDNSRecord,
  deleteDNSWorker,
  deleteDNSZone,
  disableDNSZoneDNSSEC,
  enableDNSZoneDNSSEC,
  getDNSGSLBSchedulingStates,
  getDNSMigrationCandidates,
  getDNSObservability,
  getDNSWorkers,
  getDNSZoneDNSSEC,
  getDNSZoneWorkers,
  getDNSZoneRecords,
  getDNSZones,
  probeDNSWorker,
  requestDNSWorkerUpdate,
  revokeDNSWorkerToken,
  rotateDNSZoneDNSSECKSK,
  rotateDNSZoneDNSSECZSK,
  rotateDNSWorkerToken,
  simulateDNSGSLB,
  updateDNSWorker,
  updateDNSZoneWorkers,
} from '@/features/authoritative-dns/api/authoritative-dns';
import type {
  DNSGSLBSimulationPayload,
  DNSRecordItem,
  DNSSECDenialMode,
  DNSWorkerHealthItem,
  DNSWorkerItem,
  DNSWorkerProbe,
  DNSZoneItem,
} from '@/features/authoritative-dns/types';
import {
  getProxyRoutes,
  switchProxyRouteToAuthoritativeDNS,
} from '@/features/proxy-routes/api/proxy-routes';
import { getErrorMessage } from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  PrimaryButton,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';
import { copyToClipboard } from '@/lib/utils/clipboard';
import type {
  FeedbackState,
  ActiveTab,
  WorkerSettingsFormValues,
  MigrationRecheckResult,
} from './authoritative-dns-page.helpers';
import {
  getDNSWorkerDisplayName,
  defaultDNSObservabilityWindowHours,
  dnsObservabilityWindowOptions,
  getRouteDisplayName,
  getDefaultSimulationQName,
  getRouteRecordType,
  createMigrationRecheckResult,
  updateMigrationRecheckStep,
  finalizeMigrationRecheck,
  mergeUpdatedRoute,
  getRouteSimulationCountries,
  getMigrationRecheckTone,
  shouldShowNoAuthoritativeRoutesNotice,
  getDelegationStatusLabel,
} from './authoritative-dns-page.helpers';
import {
  ZoneEditorModal,
  RecordEditorModal,
  WorkerCreateModal,
  WorkerSettingsModal,
  WorkerTokenModal,
} from './editor-modals';
import { GSLBSchedulingStatesPanel, GSLBSimulationPanel } from './gslb-panels';
import { DNSMigrationGuidePanel } from './migration-guide-panel';
import { DNSObservabilityPanel } from './observability-panel';
import { WorkersPanel } from './workers-panel';
import { ZonesPanel } from './zones-panel';

export function AuthoritativeDNSPage() {
  const queryClient = useQueryClient();
  const confirmDialog = useConfirmDialog();
  const { setFeedback } = useToastFeedback<FeedbackState>();
  const [activeTab, setActiveTab] = useState<ActiveTab>('zones');
  const [selectedZoneId, setSelectedZoneId] = useState<number | null>(null);
  const [editingZone, setEditingZone] = useState<DNSZoneItem | null>(null);
  const [isZoneModalOpen, setIsZoneModalOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<DNSRecordItem | null>(
    null,
  );
  const [recordZone, setRecordZone] = useState<DNSZoneItem | null>(null);
  const [isWorkerModalOpen, setIsWorkerModalOpen] = useState(false);
  const [workerSettingsTarget, setWorkerSettingsTarget] =
    useState<DNSWorkerHealthItem | null>(null);
  const [createdWorker, setCreatedWorker] = useState<DNSWorkerItem | null>(
    null,
  );
  const [rotatedWorker, setRotatedWorker] = useState<DNSWorkerItem | null>(
    null,
  );
  const [workerProbeResults, setWorkerProbeResults] = useState<
    Record<number, DNSWorkerProbe>
  >({});
  const [migrationRecheck, setMigrationRecheck] =
    useState<MigrationRecheckResult | null>(null);
  const [recheckingRouteId, setRecheckingRouteId] = useState<number | null>(
    null,
  );
  const [observabilityWindowKey, setObservabilityWindowKey] = useState('6h');
  const [serverUrl, setServerUrl] = useState('https://cdn.example.com');

  useEffect(() => {
    setServerUrl(window.location.origin);
  }, []);

  const zonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
  });
  const workersQuery = useQuery({
    queryKey: ['authoritative-dns', 'workers'],
    queryFn: getDNSWorkers,
  });
  const proxyRoutesQuery = useQuery({
    queryKey: ['proxy-routes'],
    queryFn: getProxyRoutes,
  });
  const observabilityWindowOption =
    dnsObservabilityWindowOptions.find(
      (option) => option.key === observabilityWindowKey,
    ) ?? dnsObservabilityWindowOptions[0];
  const observabilityWindowHours =
    observabilityWindowOption?.hours ?? defaultDNSObservabilityWindowHours;
  const observabilityQuery = useQuery({
    queryKey: ['authoritative-dns', 'observability', observabilityWindowHours],
    queryFn: () => getDNSObservability(observabilityWindowHours),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    placeholderData: (previousData) => previousData,
  });
  const schedulingStatesQuery = useQuery({
    queryKey: ['authoritative-dns', 'scheduling-states'],
    queryFn: getDNSGSLBSchedulingStates,
  });
  const migrationCandidatesQuery = useQuery({
    queryKey: ['authoritative-dns', 'migration-candidates'],
    queryFn: getDNSMigrationCandidates,
  });

  const zones = useMemo(() => zonesQuery.data ?? [], [zonesQuery.data]);
  const workers = useMemo(() => workersQuery.data ?? [], [workersQuery.data]);
  const migrationCandidates = useMemo(
    () => migrationCandidatesQuery.data ?? [],
    [migrationCandidatesQuery.data],
  );
  const proxyRoutes = useMemo(
    () => proxyRoutesQuery.data ?? [],
    [proxyRoutesQuery.data],
  );
  const observability = observabilityQuery.data ?? null;
  const schedulingStates = schedulingStatesQuery.data ?? null;
  useEffect(() => {
    if (!workerSettingsTarget || !observability?.worker_health?.workers) {
      return;
    }
    const refreshed = observability.worker_health.workers.find(
      (worker) => worker.id === workerSettingsTarget.id,
    );
    if (refreshed && refreshed !== workerSettingsTarget) {
      setWorkerSettingsTarget(refreshed);
    }
  }, [observability?.worker_health?.workers, workerSettingsTarget]);
  const authoritativeRoutes = useMemo(
    () =>
      proxyRoutes
        .filter((route) => route.dns_provider_mode === 'authoritative')
        .filter((route) => route.enabled || route.gslb_enabled),
    [proxyRoutes],
  );
  const handleCopyDNSWorkerCommand = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setFeedback({ tone: 'success', message });
    } catch (error) {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    }
  };
  const showNoAuthoritativeRoutesNotice = shouldShowNoAuthoritativeRoutesNotice(
    {
      zones,
      workers,
      routes: proxyRoutes,
      routesLoading: proxyRoutesQuery.isLoading,
      routesError: proxyRoutesQuery.isError,
    },
  );
  const selectedZone = useMemo(
    () => zones.find((zone) => zone.id === selectedZoneId) ?? zones[0] ?? null,
    [selectedZoneId, zones],
  );
  const selectedZoneRecordsQuery = useQuery({
    queryKey: ['authoritative-dns', 'zone-records', selectedZone?.id],
    queryFn: () => getDNSZoneRecords(selectedZone?.id ?? 0),
    enabled: Boolean(selectedZone?.id),
  });
  const selectedZoneWorkersQuery = useQuery({
    queryKey: ['authoritative-dns', 'zone-workers', selectedZone?.id],
    queryFn: () => getDNSZoneWorkers(selectedZone?.id ?? 0),
    enabled: Boolean(selectedZone?.id),
  });
  const selectedZoneDNSSECQuery = useQuery({
    queryKey: ['authoritative-dns', 'zone-dnssec', selectedZone?.id],
    queryFn: () => getDNSZoneDNSSEC(selectedZone?.id ?? 0),
    enabled: Boolean(selectedZone?.id),
  });
  const delegationCheckQuery = useQuery({
    queryKey: ['authoritative-dns', 'delegation-check', selectedZone?.id],
    queryFn: () => checkDNSZoneDelegation(selectedZone?.id ?? 0),
    enabled: false,
  });
  const records = useMemo(
    () => selectedZoneRecordsQuery.data ?? [],
    [selectedZoneRecordsQuery.data],
  );

  const deleteZoneMutation = useMutation({
    mutationFn: deleteDNSZone,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: '托管域名已删除。' });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'zones'],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const deleteRecordMutation = useMutation({
    mutationFn: deleteDNSRecord,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'DNS 记录已删除。' });
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zones'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zone-records'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const deleteWorkerMutation = useMutation({
    mutationFn: deleteDNSWorker,
    onSuccess: async () => {
      setFeedback({
        tone: 'success',
        message:
          '已从面板移除 DNS 响应端，并等待响应端下一次心跳执行自动卸载清理。',
      });
      setWorkerSettingsTarget(null);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'observability'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const updateWorkerMutation = useMutation({
    mutationFn: ({
      id,
      values,
    }: {
      id: number;
      values: WorkerSettingsFormValues;
    }) =>
      updateDNSWorker(id, {
        remark: values.remark.trim(),
      }),
    onSuccess: async (worker) => {
      setFeedback({
        tone: 'success',
        message: `DNS 响应端“${getDNSWorkerDisplayName(worker)}”显示名称已保存。`,
      });
      setWorkerSettingsTarget(null);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'observability'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const probeWorkerMutation = useMutation({
    mutationFn: (worker: DNSWorkerItem) =>
      probeDNSWorker(worker.id, { zone_id: selectedZone?.id }),
    onSuccess: (result, worker) => {
      setWorkerProbeResults((current) => ({
        ...current,
        [worker.id]: result,
      }));
      const reachableCount = result.results.filter(
        (item) => item.reachable,
      ).length;
      setFeedback({
        tone:
          reachableCount === result.results.length && result.results.length > 0
            ? 'success'
            : 'danger',
        message: `DNS 响应端探测完成：${reachableCount} / ${result.results.length} 可达。`,
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const requestWorkerUpdateMutation = useMutation({
    mutationFn: requestDNSWorkerUpdate,
    onSuccess: async (worker) => {
      const dispatchMessage =
        worker.update_dispatch_message ||
        '已下发 DNS Worker 更新任务。匹配到同机 Agent 时会由 Agent 执行；未匹配时回退为响应端心跳更新。';
      setFeedback({
        tone: 'success',
        message: `${getDNSWorkerDisplayName(worker)}：${dispatchMessage}`,
      });
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'observability'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const updateZoneWorkersMutation = useMutation({
    mutationFn: ({
      zoneId,
      workerIds,
    }: {
      zoneId: number;
      workerIds: number[];
    }) => updateDNSZoneWorkers(zoneId, { worker_ids: workerIds }),
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: '托管域名响应端范围已保存。' });
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zone-workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zones'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const refreshDNSSECQueries = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'zone-dnssec'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'zones'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'observability'],
      }),
    ]);
  };
  const enableDNSSECMutation = useMutation({
    mutationFn: ({
      zoneId,
      denialMode,
      nsec3Iterations,
    }: {
      zoneId: number;
      denialMode: DNSSECDenialMode;
      nsec3Iterations: number;
    }) =>
      enableDNSZoneDNSSEC(zoneId, {
        denial_mode: denialMode,
        nsec3_iterations: nsec3Iterations,
      }),
    onSuccess: async () => {
      setFeedback({
        tone: 'success',
        message: 'DNSSEC 已启用，请把 DS 记录配置到注册商。',
      });
      await refreshDNSSECQueries();
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const disableDNSSECMutation = useMutation({
    mutationFn: disableDNSZoneDNSSEC,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'DNSSEC 已关闭。' });
      await refreshDNSSECQueries();
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const rotateZSKMutation = useMutation({
    mutationFn: rotateDNSZoneDNSSECZSK,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'ZSK 已轮换。' });
      await refreshDNSSECQueries();
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const rotateKSKMutation = useMutation({
    mutationFn: rotateDNSZoneDNSSECKSK,
    onSuccess: async () => {
      setFeedback({
        tone: 'success',
        message: 'KSK 已轮换，请更新注册商 DS 记录。',
      });
      await refreshDNSSECQueries();
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const rotateWorkerTokenMutation = useMutation({
    mutationFn: rotateDNSWorkerToken,
    onSuccess: async (worker) => {
      setRotatedWorker(worker);
      setFeedback({
        tone: 'success',
        message: 'DNS 响应端密钥已重新生成，请复制新的部署密钥。',
      });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const revokeWorkerTokenMutation = useMutation({
    mutationFn: revokeDNSWorkerToken,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'DNS 响应端密钥已吊销。' });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const simulateGSLBMutation = useMutation({
    mutationFn: simulateDNSGSLB,
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const switchAuthoritativeMutation = useMutation({
    mutationFn: ({
      route,
      zone,
    }: {
      route: ProxyRouteItem;
      zone: DNSZoneItem;
    }) =>
      switchProxyRouteToAuthoritativeDNS(route.id, {
        dns_zone_id_ref: zone.id,
      }),
    onSuccess: async (updatedRoute, variables) => {
      setFeedback({
        tone: 'success',
        message: `已切换“${getRouteDisplayName(updatedRoute)}”到本地自建解析，正在自动复测。`,
      });
      queryClient.setQueryData<ProxyRouteItem[]>(['proxy-routes'], (current) =>
        mergeUpdatedRoute(current ?? proxyRoutes, updatedRoute),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['proxy-routes'] }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'migration-candidates'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'scheduling-states'],
        }),
      ]);
      await runMigrationRecheck(updatedRoute, variables.zone);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  async function runMigrationRecheck(route: ProxyRouteItem, zone: DNSZoneItem) {
    let result = updateMigrationRecheckStep(
      createMigrationRecheckResult(route, zone),
      'mode',
      'running',
      '正在刷新网站解析模式',
    );
    setMigrationRecheck(result);
    setRecheckingRouteId(route.id);
    setFeedback({
      tone: 'info',
      message: `正在复测“${getRouteDisplayName(route)}”的本地自建解析切换结果。`,
    });

    try {
      let latestRoute = route;
      let latestRoutes = proxyRoutes;
      try {
        latestRoutes = await queryClient.fetchQuery({
          queryKey: ['proxy-routes'],
          queryFn: getProxyRoutes,
        });
        latestRoute =
          latestRoutes.find((item) => item.id === route.id) ?? route;
        if (
          latestRoute.dns_provider_mode !== 'authoritative' &&
          route.dns_provider_mode === 'authoritative' &&
          route.dns_zone_id_ref === zone.id
        ) {
          latestRoute = route;
        }
        result = updateMigrationRecheckStep(
          result,
          'mode',
          latestRoute.dns_provider_mode === 'authoritative' &&
            latestRoute.dns_zone_id_ref === zone.id
            ? 'success'
            : 'danger',
          latestRoute.dns_provider_mode === 'authoritative'
            ? `已绑定托管域名 ${zone.name}`
            : '网站尚未切换到本地自建解析',
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'mode',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);

      result = updateMigrationRecheckStep(
        result,
        'delegation',
        'running',
        '正在检查注册商指向',
      );
      setMigrationRecheck(result);
      try {
        const delegationCheck = await checkDNSZoneDelegation(zone.id);
        result = {
          ...result,
          delegationCheck,
        };
        const delegationStatus =
          delegationCheck.status === 'matched'
            ? 'success'
            : delegationCheck.status === 'failed' ||
                delegationCheck.status === 'mismatch'
              ? 'danger'
              : 'warning';
        result = updateMigrationRecheckStep(
          result,
          'delegation',
          delegationStatus,
          delegationCheck.status === 'matched'
            ? '公网解析服务器已与托管域名配置匹配'
            : `当前委派状态：${getDelegationStatusLabel(delegationCheck.status)}`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'delegation',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);

      result = updateMigrationRecheckStep(
        result,
        'worker_probe',
        'running',
        '正在探测在线 DNS 响应端的 UDP/TCP 53',
      );
      setMigrationRecheck(result);
      try {
        let latestWorkers =
          (await queryClient.fetchQuery({
            queryKey: ['authoritative-dns', 'workers'],
            queryFn: getDNSWorkers,
          })) ?? workers;
        if (latestWorkers.length === 0 && workers.length > 0) {
          latestWorkers = workers;
        }
        const onlineWorkersForProbe = latestWorkers.filter(
          (worker) => worker.status === 'online',
        );
        const workerProbePairs = await Promise.allSettled(
          onlineWorkersForProbe.map(async (worker) => ({
            worker,
            probe: await probeDNSWorker(worker.id, { zone_id: zone.id }),
          })),
        );
        const successfulWorkerProbes: DNSWorkerProbe[] = [];
        const nextWorkerProbeResults: Record<number, DNSWorkerProbe> = {};
        for (const item of workerProbePairs) {
          if (item.status === 'fulfilled') {
            successfulWorkerProbes.push(item.value.probe);
            nextWorkerProbeResults[item.value.worker.id] = item.value.probe;
          }
        }
        if (Object.keys(nextWorkerProbeResults).length > 0) {
          setWorkerProbeResults((current) => ({
            ...current,
            ...nextWorkerProbeResults,
          }));
        }
        const healthyProbeCount = successfulWorkerProbes.filter(
          (probe) =>
            probe.results.length > 0 &&
            probe.results.every((probeResult) => probeResult.reachable),
        ).length;
        result = {
          ...result,
          workerProbes: successfulWorkerProbes,
        };
        result = updateMigrationRecheckStep(
          result,
          'worker_probe',
          healthyProbeCount > 0
            ? healthyProbeCount === onlineWorkersForProbe.length
              ? 'success'
              : 'warning'
            : 'danger',
          onlineWorkersForProbe.length === 0
            ? '没有在线 DNS 响应端'
            : `${healthyProbeCount} / ${onlineWorkersForProbe.length} 个在线响应端 UDP/TCP 53 可达`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'worker_probe',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'observability'],
      });

      result = updateMigrationRecheckStep(
        result,
        'simulation',
        'running',
        '正在按当前解析配置模拟全局和来源国家的返回 IP',
      );
      setMigrationRecheck(result);
      try {
        const routeForSimulation = {
          ...latestRoute,
          dns_provider_mode: 'authoritative' as const,
          dns_zone_id_ref: zone.id,
        };
        const routeListForSimulation = mergeUpdatedRoute(
          latestRoutes,
          routeForSimulation,
        );
        queryClient.setQueryData(['proxy-routes'], routeListForSimulation);
        const simulationCountries =
          getRouteSimulationCountries(routeForSimulation);
        const simulationResults = await Promise.all(
          simulationCountries.map((country) =>
            simulateDNSGSLB({
              proxy_route_id: routeForSimulation.id,
              qname: getDefaultSimulationQName(routeForSimulation),
              record_type: getRouteRecordType(routeForSimulation),
              country,
              source_ip: '',
              fresh: true,
            }),
          ),
        );
        result = {
          ...result,
          simulations: simulationResults,
        };
        const returnedTargetCount = simulationResults.reduce(
          (count, simulation) => count + simulation.targets.length,
          0,
        );
        const noTargetSimulationCount = simulationResults.filter(
          (simulation) => simulation.targets.length === 0,
        ).length;
        result = updateMigrationRecheckStep(
          result,
          'simulation',
          returnedTargetCount === 0
            ? 'danger'
            : noTargetSimulationCount > 0
              ? 'warning'
              : 'success',
          returnedTargetCount === 0
            ? '当前解析配置没有返回可用目标'
            : noTargetSimulationCount > 0
              ? `已完成 ${simulationResults.length} 组模拟，其中 ${noTargetSimulationCount} 组无返回目标`
              : `已完成 ${simulationResults.length} 组模拟，返回 ${returnedTargetCount} 个目标`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'simulation',
          'danger',
          getErrorMessage(error),
        );
      }

      result = finalizeMigrationRecheck(result);
      setMigrationRecheck(result);
      setFeedback({
        tone: getMigrationRecheckTone(result.status),
        message:
          result.status === 'success'
            ? `“${result.routeName}”切换后复测通过。`
            : `“${result.routeName}”切换后复测完成，请查看迁移向导右侧结果。`,
      });
    } finally {
      setRecheckingRouteId(null);
    }
  }

  const openCreateZone = () => {
    setEditingZone(null);
    setIsZoneModalOpen(true);
  };
  const openEditZone = (zone: DNSZoneItem) => {
    setEditingZone(zone);
    setIsZoneModalOpen(true);
  };
  const openCreateRecord = (zone: DNSZoneItem) => {
    setRecordZone(zone);
    setEditingRecord(null);
  };
  const openEditRecord = (zone: DNSZoneItem, record: DNSRecordItem) => {
    setRecordZone(zone);
    setEditingRecord(record);
  };

  const handleDeleteZone = async (zone: DNSZoneItem) => {
    const confirmed = await confirmDialog({
      title: '删除托管域名',
      message: `确认删除托管域名“${zone.name}”吗？会同时删除该域名下的静态记录；已被网站本地自建解析引用时后端会阻止删除。`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteZoneMutation.mutate(zone.id);
    }
  };

  const handleDeleteRecord = async (record: DNSRecordItem) => {
    const confirmed = await confirmDialog({
      title: '删除 DNS 记录',
      message: `确认删除 ${record.name} 的 ${record.type} 记录吗？`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteRecordMutation.mutate(record.id);
    }
  };

  const handleDeleteWorker = async (
    worker: DNSWorkerItem | DNSWorkerHealthItem,
  ) => {
    if (!worker.uninstall_supported) {
      setFeedback({
        tone: 'info',
        message:
          '该 DNS 响应端当前版本不支持远程卸载，请先强制更新一次，或登录机器手动执行 uninstall-dns-worker.sh。',
      });
      return;
    }
    const confirmed = await confirmDialog({
      title: '删除 DNS 响应端',
      message: `确认删除响应端“${getDNSWorkerDisplayName(worker)}”吗？面板会先隐藏该响应端，并在下一次心跳下发自动卸载清理命令；如果响应端已经离线，命令会等到它恢复心跳后执行。`,
      confirmLabel: '删除并卸载',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteWorkerMutation.mutate(worker.id);
    }
  };

  const handleRotateWorkerToken = async (worker: DNSWorkerItem) => {
    const confirmed = await confirmDialog({
      title: '重新生成响应端密钥',
      message: `确定要为“${getDNSWorkerDisplayName(worker)}”重新生成密钥吗？旧密钥会立即失效，已经部署的响应端需要更新为新密钥。`,
      confirmLabel: '重新生成',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      rotateWorkerTokenMutation.mutate(worker.id);
    }
  };

  const handleRevokeWorkerToken = async (worker: DNSWorkerItem) => {
    const confirmed = await confirmDialog({
      title: '吊销响应端密钥',
      message: `确定要吊销“${getDNSWorkerDisplayName(worker)}”的当前密钥吗？吊销后该响应端不能继续拉取解析配置，直到重新生成并部署新密钥。`,
      confirmLabel: '吊销密钥',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      revokeWorkerTokenMutation.mutate(worker.id);
    }
  };

  const handleProbeWorker = (worker: DNSWorkerItem) => {
    setFeedback(null);
    probeWorkerMutation.mutate(worker);
  };

  const handleSimulateGSLB = (payload: DNSGSLBSimulationPayload) => {
    setFeedback(null);
    simulateGSLBMutation.mutate(payload);
  };

  const handleSwitchAuthoritative = async (
    route: ProxyRouteItem,
    zone: DNSZoneItem,
  ) => {
    const confirmed = await confirmDialog({
      title: '切换到本地自建解析',
      message: `确认把“${getRouteDisplayName(route)}”切换到本地自建解析，并绑定托管域名“${zone.name}”吗？切换后请到注册商确认 NS 已指向你的 DNS 响应端。`,
      confirmLabel: '切换',
    });
    if (confirmed) {
      setFeedback(null);
      switchAuthoritativeMutation.mutate({ route, zone });
    }
  };

  if (zonesQuery.isLoading || workersQuery.isLoading) {
    return <LoadingState />;
  }

  if (zonesQuery.isError) {
    return (
      <ErrorState
        title="托管域名加载失败"
        description={getErrorMessage(zonesQuery.error)}
      />
    );
  }

  if (workersQuery.isError) {
    return (
      <ErrorState
        title="DNS 响应端加载失败"
        description={getErrorMessage(workersQuery.error)}
      />
    );
  }

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="本地自建解析"
          description="管理本地自建解析的托管域名、静态记录和 DNS 响应端，用于按访问来源与节点状态自动返回合适的边缘 IP。"
          action={
            <div className="flex flex-wrap gap-3">
              <SecondaryButton
                type="button"
                onClick={() => setIsWorkerModalOpen(true)}
              >
                创建 DNS 响应端
              </SecondaryButton>
              <PrimaryButton type="button" onClick={openCreateZone}>
                创建托管域名
              </PrimaryButton>
            </div>
          }
        />

        <div className="grid gap-4 md:grid-cols-3">
          <AppCard title="托管域名">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {zones.length}
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                启用 {zones.filter((zone) => zone.enabled).length} 个
              </p>
            </div>
          </AppCard>
          <AppCard title="静态记录">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {zones
                  .reduce((sum, zone) => sum + zone.record_count, 0)
                  .toLocaleString('zh-CN')}
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                不含网站自动返回的动态记录
              </p>
            </div>
          </AppCard>
          <AppCard title="DNS 响应端">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {workers.filter((worker) => worker.status === 'online').length}
                <span className="text-base text-[var(--foreground-secondary)]">
                  {' '}
                  / {workers.length}
                </span>
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                在线 / 全部响应端
              </p>
            </div>
          </AppCard>
        </div>

        {showNoAuthoritativeRoutesNotice ? (
          <InlineMessage
            tone="warning"
            message="DNS 响应端已经能拉取解析配置，但当前没有启用的网站绑定到本地自建解析。此时响应端只能回答基础记录和静态记录；业务域名需要到网站详情「负载均衡」切换为本地自建解析并选择对应托管域名，或使用迁移向导一键切换。"
          />
        ) : null}

        <DNSObservabilityPanel
          summary={observability}
          isLoading={observabilityQuery.isLoading}
          error={
            observabilityQuery.isError
              ? getErrorMessage(observabilityQuery.error)
              : ''
          }
          selectedWindowKey={observabilityWindowKey}
          selectedWindowHours={observabilityWindowHours}
          onWindowChange={setObservabilityWindowKey}
          onCopyCommand={handleCopyDNSWorkerCommand}
          onOpenWorkerSettings={setWorkerSettingsTarget}
        />

        <GSLBSimulationPanel
          routes={authoritativeRoutes}
          routesLoading={proxyRoutesQuery.isLoading}
          routesError={
            proxyRoutesQuery.isError
              ? getErrorMessage(proxyRoutesQuery.error)
              : ''
          }
          result={simulateGSLBMutation.data ?? null}
          error={
            simulateGSLBMutation.isError
              ? getErrorMessage(simulateGSLBMutation.error)
              : ''
          }
          isPending={simulateGSLBMutation.isPending}
          onSimulate={handleSimulateGSLB}
        />

        <div className="flex flex-wrap gap-3">
          {[
            {
              key: 'zones' as const,
              label: '托管域名与记录',
              description: '托管域名、注册商 NS 和静态 DNS 记录。',
            },
            {
              key: 'workers' as const,
              label: 'DNS 响应端',
              description: '管理对外回答 DNS 查询的服务和配置状态。',
            },
            {
              key: 'migration' as const,
              label: '迁移向导',
              description: '检查 Cloudflare 站点切换到本地自建解析的准备项。',
            },
          ].map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'rounded-2xl border px-4 py-3 text-left transition',
                activeTab === tab.key
                  ? 'border-[var(--border-strong)] bg-[var(--accent-soft)] text-[var(--foreground-primary)]'
                  : 'border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--border-strong)] hover:text-[var(--foreground-primary)]',
              )}
            >
              <p className="text-sm font-semibold">{tab.label}</p>
              <p className="mt-1 text-xs leading-5 text-inherit/80">
                {tab.description}
              </p>
            </button>
          ))}
        </div>

        {activeTab === 'zones' ? (
          <ZonesPanel
            zones={zones}
            selectedZone={selectedZone}
            records={records}
            recordsLoading={selectedZoneRecordsQuery.isLoading}
            recordsError={
              selectedZoneRecordsQuery.isError
                ? getErrorMessage(selectedZoneRecordsQuery.error)
                : ''
            }
            delegationCheck={delegationCheckQuery.data ?? null}
            delegationLoading={delegationCheckQuery.isFetching}
            delegationError={
              delegationCheckQuery.isError
                ? getErrorMessage(delegationCheckQuery.error)
                : ''
            }
            workers={workers}
            zoneWorkerAssignment={selectedZoneWorkersQuery.data ?? null}
            zoneWorkerAssignmentLoading={selectedZoneWorkersQuery.isLoading}
            zoneWorkerAssignmentError={
              selectedZoneWorkersQuery.isError
                ? getErrorMessage(selectedZoneWorkersQuery.error)
                : ''
            }
            zoneWorkerAssignmentSaving={updateZoneWorkersMutation.isPending}
            dnssec={selectedZoneDNSSECQuery.data ?? null}
            dnssecLoading={selectedZoneDNSSECQuery.isLoading}
            dnssecError={
              selectedZoneDNSSECQuery.isError
                ? getErrorMessage(selectedZoneDNSSECQuery.error)
                : ''
            }
            dnssecBusy={
              enableDNSSECMutation.isPending ||
              disableDNSSECMutation.isPending ||
              rotateZSKMutation.isPending ||
              rotateKSKMutation.isPending
            }
            onCheckDelegation={() => {
              setFeedback(null);
              void delegationCheckQuery.refetch();
            }}
            onSaveZoneWorkers={(zone, workerIds) =>
              updateZoneWorkersMutation.mutate({
                zoneId: zone.id,
                workerIds,
              })
            }
            onEnableDNSSEC={(zone, denialMode, nsec3Iterations) =>
              enableDNSSECMutation.mutate({
                zoneId: zone.id,
                denialMode,
                nsec3Iterations,
              })
            }
            onDisableDNSSEC={(zone) =>
              void confirmDialog({
                title: '关闭 DNSSEC',
                message:
                  '关闭后该托管域名不再返回 DNSSEC 签名。请先确认注册商 DS 记录已经移除，否则外部验证解析可能失败。',
                confirmLabel: '关闭 DNSSEC',
                tone: 'danger',
              }).then((confirmed) => {
                if (confirmed) {
                  disableDNSSECMutation.mutate(zone.id);
                }
              })
            }
            onRotateZSK={(zone) =>
              void confirmDialog({
                title: '轮换 ZSK',
                message:
                  'ZSK 轮换会更新在线签名密钥，响应端下一次拉取快照后生效。',
                confirmLabel: '轮换 ZSK',
              }).then((confirmed) => {
                if (confirmed) {
                  rotateZSKMutation.mutate(zone.id);
                }
              })
            }
            onRotateKSK={(zone) =>
              void confirmDialog({
                title: '轮换 KSK',
                message:
                  'KSK 轮换会生成新的 DS 记录。请准备在注册商同步更新 DS，避免验证链中断。',
                confirmLabel: '轮换 KSK',
              }).then((confirmed) => {
                if (confirmed) {
                  rotateKSKMutation.mutate(zone.id);
                }
              })
            }
            onSelectZone={(zone) => setSelectedZoneId(zone.id)}
            onCreateZone={openCreateZone}
            onEditZone={openEditZone}
            onDeleteZone={handleDeleteZone}
            onCreateRecord={openCreateRecord}
            onEditRecord={openEditRecord}
            onDeleteRecord={handleDeleteRecord}
            busy={
              deleteZoneMutation.isPending || deleteRecordMutation.isPending
            }
          />
        ) : activeTab === 'workers' ? (
          <WorkersPanel
            workers={workers}
            onCreateWorker={() => setIsWorkerModalOpen(true)}
            onProbeWorker={handleProbeWorker}
            busy={deleteWorkerMutation.isPending}
            rotatingWorkerId={
              rotateWorkerTokenMutation.isPending
                ? (rotateWorkerTokenMutation.variables ?? null)
                : null
            }
            revokingWorkerId={
              revokeWorkerTokenMutation.isPending
                ? (revokeWorkerTokenMutation.variables ?? null)
                : null
            }
            probingWorkerId={
              probeWorkerMutation.isPending
                ? (probeWorkerMutation.variables?.id ?? null)
                : null
            }
            probeResults={workerProbeResults}
            onRotateToken={handleRotateWorkerToken}
            onRevokeToken={handleRevokeWorkerToken}
          />
        ) : (
          <DNSMigrationGuidePanel
            routes={proxyRoutes}
            migrationCandidates={migrationCandidates}
            zones={zones}
            workers={workers}
            routesLoading={
              proxyRoutesQuery.isLoading || migrationCandidatesQuery.isLoading
            }
            routesError={
              proxyRoutesQuery.isError
                ? getErrorMessage(proxyRoutesQuery.error)
                : migrationCandidatesQuery.isError
                  ? getErrorMessage(migrationCandidatesQuery.error)
                  : ''
            }
            switchingRouteId={
              switchAuthoritativeMutation.isPending
                ? switchAuthoritativeMutation.variables.route.id
                : null
            }
            recheckingRouteId={recheckingRouteId}
            recheckResult={migrationRecheck}
            onSwitchAuthoritative={handleSwitchAuthoritative}
          />
        )}

        <GSLBSchedulingStatesPanel
          states={schedulingStates}
          isLoading={schedulingStatesQuery.isLoading}
          error={
            schedulingStatesQuery.isError
              ? getErrorMessage(schedulingStatesQuery.error)
              : ''
          }
        />
      </div>

      {isZoneModalOpen ? (
        <ZoneEditorModal
          isOpen={isZoneModalOpen}
          zone={editingZone}
          onClose={() => setIsZoneModalOpen(false)}
          onSaved={async (zone) => {
            setSelectedZoneId(zone.id);
            setIsZoneModalOpen(false);
            setFeedback({
              tone: 'success',
              message: editingZone ? '托管域名已保存。' : '托管域名已创建。',
            });
            await queryClient.invalidateQueries({
              queryKey: ['authoritative-dns', 'zones'],
            });
          }}
        />
      ) : null}

      {recordZone ? (
        <RecordEditorModal
          isOpen={Boolean(recordZone)}
          zone={recordZone}
          record={editingRecord}
          onClose={() => {
            setRecordZone(null);
            setEditingRecord(null);
          }}
          onSaved={async () => {
            setRecordZone(null);
            setEditingRecord(null);
            setFeedback({
              tone: 'success',
              message: editingRecord ? 'DNS 记录已保存。' : 'DNS 记录已创建。',
            });
            await Promise.all([
              queryClient.invalidateQueries({
                queryKey: ['authoritative-dns', 'zones'],
              }),
              queryClient.invalidateQueries({
                queryKey: ['authoritative-dns', 'zone-records'],
              }),
            ]);
          }}
        />
      ) : null}

      {isWorkerModalOpen ? (
        <WorkerCreateModal
          isOpen={isWorkerModalOpen}
          onClose={() => setIsWorkerModalOpen(false)}
          onCreated={async (worker) => {
            setIsWorkerModalOpen(false);
            setCreatedWorker(worker);
            setFeedback({ tone: 'success', message: 'DNS 响应端已创建。' });
            await queryClient.invalidateQueries({
              queryKey: ['authoritative-dns', 'workers'],
            });
          }}
        />
      ) : null}

      {createdWorker ? (
        <WorkerTokenModal
          worker={createdWorker}
          serverUrl={serverUrl}
          title="DNS 响应端密钥"
          description={`响应端 ${getDNSWorkerDisplayName(createdWorker)} 已创建。密钥离开弹窗后不会再次显示。`}
          onClose={() => setCreatedWorker(null)}
        />
      ) : null}

      {rotatedWorker ? (
        <WorkerTokenModal
          worker={rotatedWorker}
          serverUrl={serverUrl}
          title="新的 DNS 响应端密钥"
          description={`响应端 ${getDNSWorkerDisplayName(rotatedWorker)} 的密钥已重新生成。旧密钥已失效，请复制新密钥并更新部署。`}
          onClose={() => setRotatedWorker(null)}
        />
      ) : null}

      {workerSettingsTarget ? (
        <WorkerSettingsModal
          worker={workerSettingsTarget}
          isSaving={updateWorkerMutation.isPending}
          isRequestingUpdate={
            requestWorkerUpdateMutation.isPending &&
            requestWorkerUpdateMutation.variables === workerSettingsTarget.id
          }
          isDeleting={
            deleteWorkerMutation.isPending &&
            deleteWorkerMutation.variables === workerSettingsTarget.id
          }
          onClose={() => setWorkerSettingsTarget(null)}
          onSave={(values) =>
            updateWorkerMutation.mutate({
              id: workerSettingsTarget.id,
              values,
            })
          }
          onRequestUpdate={() =>
            requestWorkerUpdateMutation.mutate(workerSettingsTarget.id)
          }
          onDelete={() => handleDeleteWorker(workerSettingsTarget)}
        />
      ) : null}
    </>
  );
}
