import {
  Component,
  EventEmitter,
  Input,
  OnDestroy,
  OnInit,
  Output,
} from '@angular/core';
import {
  AbstractControl,
  FormBuilder,
  FormGroup,
  ValidationErrors,
  Validators,
} from '@angular/forms';
import { V1EnvVar, V1Pod, V1Volume } from '@kubernetes/client-node';
import { ChipDescriptor, PollerService, STATUS_TYPE, UrlItem } from 'kubeflow';
import { Subscription } from 'rxjs';
import { JWABackendService } from 'src/app/services/backend.service';
import { GPUVendor, NotebookRawObject, PodDefault } from 'src/app/types';
import { EnvironmentVariablesGroup } from '../../../types/environmentVariablesGroup';
import { Configuration } from 'src/app/types/configuration';
import { isEqual, get } from 'lodash-es';
import { VolumesGroup } from './volumes/types';
import {
  PVCS,
  EPHEMERALS,
  SECRETS,
  CONFIG_MAPS,
  OTHER_VOLS,
  MEMORY_VOLS,
} from './volumes.constants';

@Component({
  selector: 'app-overview',
  templateUrl: './overview.component.html',
  styleUrls: ['./overview.component.scss'],
})
export class OverviewComponent implements OnInit, OnDestroy {
  public volGroups: VolumesGroup[] = [];
  public notebookInfoLoaded = false;
  public editingField: string | null = null;
  public isSavingResources = false;
  public resourceError = '';

  get isEditingResources(): boolean {
    return this.editingField !== null;
  }
  public resourceForm: FormGroup;
  public gpuVendors: GPUVendor[] = [];
  public readonly gpuCounts = ['none', '1', '2', '4', '8'];
  private notebookEnv: ChipDescriptor[];
  public configurations: Configuration[] = [];
  private podDefaults: PodDefault[];
  public envGroups: EnvironmentVariablesGroup[] = [];
  private prvSshNodePort: number | null = null;

  private prvNotebook: NotebookRawObject;
  private prvPod: V1Pod;
  private pollSub = new Subscription();
  private sshNodePortSub = new Subscription();

  @Input() notebookStatus: STATUS_TYPE;
  @Output() resourceUpdated = new EventEmitter<void>();

  @Input()
  set notebook(nb: NotebookRawObject) {
    this.prvNotebook = nb;

    this.notebookInfoLoaded = true;
    this.volGroups = this.generateVolGroups(nb);
    this.generatePodDefaults(nb);
    this.notebookEnv = this.generateEnv(nb);
    this.fetchSshNodePort(nb);
    this.syncResourceForm(nb);
  }
  get notebook(): NotebookRawObject {
    return this.prvNotebook;
  }

  @Input()
  set pod(pod: V1Pod) {
    if (!pod) {
      this.prvPod = pod;
      this.removePodEnvGroups();
      return;
    }

    this.prvPod = pod;
    this.generatePodEnv(pod);
  }
  get pod(): V1Pod {
    return this.prvPod;
  }

  @Input() podRequestCompleted = false;

  get podDefaultsMessage() {
    return this.getPodDefaultsMessage(
      this.podRequestCompleted,
      this.configurations,
    );
  }

  getPodDefaultsMessage(
    podRequestCompleted: boolean,
    configurations: Configuration[],
  ) {
    if (podRequestCompleted === true && configurations.length === 0) {
      return 'No configurations available for this notebook.';
    }
  }

  get notebookType(): string {
    return this.getNotebookType(this.notebook);
  }

  getNotebookType(notebook: NotebookRawObject): string {
    if (
      !notebook?.metadata?.annotations['notebooks.kubeflow.org/server-type']
    ) {
      return 'empty';
    }
    const type =
      notebook.metadata.annotations['notebooks.kubeflow.org/server-type'];
    if (type === 'group-two') {
      return 'RStudio';
    } else if (type === 'group-one') {
      return 'VSCode';
    } else if (type === 'jupyter') {
      return 'JupyterLab';
    } else {
      return type;
    }
  }

  get podDefaultsNotEmpty() {
    return this.getPodDefaultsNotEmpty(this.podDefaults);
  }

  getPodDefaultsNotEmpty(podDefaults: PodDefault[]) {
    if (podDefaults?.length === 0) {
      return false;
    }
    return true;
  }

  get sharedMemory(): string {
    return this.getSharedMemory(this.notebook);
  }

  getSharedMemory(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.volumes) {
      return 'null';
    }
    for (const volume of notebook.spec.template.spec.volumes) {
      if (volume.name === 'dshm') {
        return 'Yes';
      }
    }
    return 'No';
  }

  get cpuLimits(): string {
    return this.getCpuLimits(this.notebook);
  }

  getCpuLimits(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.containers) {
      return null;
    }
    for (const cn of notebook.spec.template.spec.containers) {
      if (cn.name !== notebook.metadata.name) {
        continue;
      }
      if (!cn.resources.limits) {
        return null;
      }
      return cn.resources.limits?.cpu;
    }
  }

  get notebookCreator(): string {
    return this.getNotebookCreator(this.notebook);
  }

  getNotebookCreator(notebook: NotebookRawObject): string {
    if (!notebook?.metadata?.annotations) {
      return null;
    }
    return notebook.metadata.annotations['notebooks.kubeflow.org/creator'];
  }

  get cpuRequests(): string {
    return this.getCpuRequest(this.notebook);
  }

  getCpuRequest(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.containers) {
      return null;
    }
    for (const cn of notebook.spec.template.spec.containers) {
      if (cn.name !== notebook.metadata.name) {
        continue;
      }
      if (!cn.resources.requests) {
        return null;
      }
      return cn.resources.requests?.cpu;
    }
  }

  get memoryRequests(): string {
    return this.getMemoryRequests(this.notebook);
  }

  getMemoryRequests(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.containers) {
      return null;
    }
    for (const cn of notebook.spec.template.spec.containers) {
      if (cn.name !== notebook.metadata.name) {
        continue;
      }
      if (!cn.resources.requests) {
        return null;
      }
      return cn.resources.requests?.memory;
    }
  }

  get memoryLimits(): string {
    return this.getMemoryLimits(this.notebook);
  }

  getMemoryLimits(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.containers) {
      return null;
    }
    for (const cn of notebook.spec.template.spec.containers) {
      if (cn.name !== notebook.metadata.name) {
        continue;
      }
      if (!cn.resources.requests) {
        return null;
      }
      return cn.resources.limits?.memory;
    }
  }

  get dockerImage(): string {
    return this.getDockerImage(this.notebook);
  }

  getDockerImage(notebook: NotebookRawObject): string {
    if (!notebook?.spec?.template?.spec?.containers) {
      return null;
    }
    for (const container of notebook.spec.template.spec.containers) {
      if (container.name === notebook.metadata.name) {
        return container.image;
      }
    }
  }

  get sshNodePort(): number | null {
    return this.getSshNodePort();
  }

  get gpuMessage(): string {
    const gpu = this.getGpuSelection(this.notebook);
    if (gpu.num === 'none') {
      return null;
    }

    return `${gpu.num} ${this.getGpuVendorName(gpu.vendor)}`;
  }

  getSshNodePort(): number | null {
    return this.prvSshNodePort;
  }

  constructor(
    private fb: FormBuilder,
    public backend: JWABackendService,
    public poller: PollerService,
  ) {
    this.resourceForm = this.fb.group(
      {
        cpu: [null, [Validators.required, Validators.min(0.1)]],
        cpuLimit: [null, [Validators.min(0.1)]],
        memory: [null, [Validators.required, Validators.min(0.1)]],
        memoryLimit: [null, [Validators.min(0.1)]],
        gpuNum: ['none'],
        gpuVendor: [''],
      },
      { validators: this.resourceLimitValidator },
    );
  }

  ngOnInit(): void {
    this.backend.getConfig().subscribe(config => {
      const vendors = (((config as any)?.gpus?.value?.vendors) || []) as GPUVendor[];
      this.gpuVendors = vendors;
      this.syncResourceForm(this.notebook);
    });

    this.gpuNumControl.valueChanges.subscribe((num: string) => {
      if (num === 'none') {
        this.gpuVendorControl.setValue('');
        this.gpuVendorControl.disable();
      } else {
        this.gpuVendorControl.enable();
      }
    });
  }

  ngOnDestroy(): void {
    if (this.pollSub) {
      this.pollSub.unsubscribe();
    }
    if (this.sshNodePortSub) {
      this.sshNodePortSub.unsubscribe();
    }
  }

  startResourceEdit(field: string) {
    this.resourceError = '';
    this.editingField = field;
    this.syncResourceForm(this.notebook);
  }

  cancelResourceEdit() {
    this.editingField = null;
    this.resourceError = '';
    this.syncResourceForm(this.notebook);
  }

  saveResources() {
    if (!this.notebook?.metadata?.namespace || !this.notebook?.metadata?.name) {
      return;
    }

    if (this.resourceForm.invalid) {
      this.resourceForm.markAllAsTouched();
      return;
    }

    const formValue = this.resourceForm.getRawValue();
    this.isSavingResources = true;
    this.resourceError = '';

    this.backend
      .updateNotebookResources(
        this.notebook.metadata.namespace,
        this.notebook.metadata.name,
        {
          cpu: String(formValue.cpu),
          cpuLimit: this.toPatchString(formValue.cpuLimit),
          memory: `${formValue.memory}Gi`,
          memoryLimit: this.toMemoryPatchString(formValue.memoryLimit),
          gpus: {
            num: formValue.gpuNum,
            vendor: formValue.gpuVendor,
          },
        },
      )
      .subscribe(
        () => {
          this.applyResourceValuesToNotebook(formValue);
          this.isSavingResources = false;
          this.editingField = null;
          this.resourceUpdated.emit();
        },
        error => {
          this.isSavingResources = false;
          this.resourceError = error;
        },
      );
  }

  get resourceFormHasLimitError() {
    return (
      this.resourceForm.hasError('cpuLimitTooSmall') ||
      this.resourceForm.hasError('memoryLimitTooSmall')
    );
  }

  get cpuControl() {
    return this.resourceForm.get('cpu');
  }

  get cpuLimitControl() {
    return this.resourceForm.get('cpuLimit');
  }

  get memoryControl() {
    return this.resourceForm.get('memory');
  }

  get memoryLimitControl() {
    return this.resourceForm.get('memoryLimit');
  }

  get gpuNumControl() {
    return this.resourceForm.get('gpuNum');
  }

  get gpuVendorControl() {
    return this.resourceForm.get('gpuVendor');
  }

  private resourceLimitValidator = (
    control: AbstractControl,
  ): ValidationErrors | null => {
    const cpu = Number(control.get('cpu')?.value);
    const cpuLimit = Number(control.get('cpuLimit')?.value);
    const memory = Number(control.get('memory')?.value);
    const memoryLimit = Number(control.get('memoryLimit')?.value);

    const errors: ValidationErrors = {};

    if (!isNaN(cpu) && !isNaN(cpuLimit) && cpuLimit < cpu) {
      errors.cpuLimitTooSmall = true;
    }

    if (!isNaN(memory) && !isNaN(memoryLimit) && memoryLimit < memory) {
      errors.memoryLimitTooSmall = true;
    }

    if (control.get('gpuNum')?.value !== 'none' && !control.get('gpuVendor')?.value) {
      errors.gpuVendorRequired = true;
    }

    return Object.keys(errors).length ? errors : null;
  };

  private syncResourceForm(notebook: NotebookRawObject) {
    const gpu = this.getGpuSelection(notebook);
    this.resourceForm.reset({
      cpu: this.parseResourceValue(this.getCpuRequest(notebook)),
      cpuLimit: this.parseResourceValue(this.getCpuLimits(notebook)),
      memory: this.parseResourceValue(this.getMemoryRequests(notebook)),
      memoryLimit: this.parseResourceValue(this.getMemoryLimits(notebook)),
      gpuNum: gpu.num,
      gpuVendor: gpu.vendor,
    });

    if (gpu.num === 'none') {
      this.gpuVendorControl.disable();
    } else {
      this.gpuVendorControl.enable();
    }
  }

  private parseResourceValue(value: string): number | null {
    if (!value) {
      return null;
    }

    const normalized = String(value).replace('Gi', '');
    const parsed = Number(normalized);

    return isNaN(parsed) ? null : parsed;
  }

  private toPatchString(value: number | string): string {
    if (value == null || value === '') {
      return '';
    }

    return String(value);
  }

  private toMemoryPatchString(value: number | string): string {
    const normalized = this.toPatchString(value);
    if (normalized === '') {
      return '';
    }

    return `${normalized}Gi`;
  }

  private applyResourceValuesToNotebook(formValue: {
    cpu: number | string;
    cpuLimit?: number | string;
    memory: number | string;
    memoryLimit?: number | string;
    gpuNum?: string;
    gpuVendor?: string;
  }) {
    const container = this.notebook?.spec?.template?.spec?.containers?.find(
      cn => cn.name === this.notebook.metadata.name,
    );

    if (!container) {
      return;
    }

    container.resources = container.resources || {};
    container.resources.requests = container.resources.requests || {};
    container.resources.limits = container.resources.limits || {};

    container.resources.requests.cpu = String(formValue.cpu);
    container.resources.requests.memory = `${formValue.memory}Gi`;

    if (formValue.cpuLimit == null || formValue.cpuLimit === '') {
      delete container.resources.limits.cpu;
    } else {
      container.resources.limits.cpu = String(formValue.cpuLimit);
    }

    if (formValue.memoryLimit == null || formValue.memoryLimit === '') {
      delete container.resources.limits.memory;
    } else {
      container.resources.limits.memory = `${formValue.memoryLimit}Gi`;
    }

    for (const vendor of this.gpuVendors) {
      delete container.resources.limits[vendor.limitsKey];
    }

    if (
      formValue.gpuNum != null &&
      formValue.gpuNum !== 'none' &&
      formValue.gpuVendor
    ) {
      container.resources.limits[formValue.gpuVendor] = String(formValue.gpuNum);
    }
  }

  private getGpuSelection(notebook: NotebookRawObject): {
    num: string;
    vendor: string;
  } {
    const container = notebook?.spec?.template?.spec?.containers?.find(
      cn => cn.name === notebook?.metadata?.name,
    );
    const limits = container?.resources?.limits || {};
    const knownGpuKeys = this.gpuVendors.map(v => v.limitsKey);

    for (const key of Object.keys(limits)) {
      if (key === 'cpu' || key === 'memory') {
        continue;
      }

      if (knownGpuKeys.length === 0 || knownGpuKeys.includes(key)) {
        return {
          num: String(limits[key]),
          vendor: key,
        };
      }
    }

    return {
      num: 'none',
      vendor: '',
    };
  }

  private getGpuVendorName(vendorKey: string): string {
    return this.gpuVendors.find(v => v.limitsKey === vendorKey)?.uiName || vendorKey;
  }

  private fetchSshNodePort(nb: NotebookRawObject) {
    if (!nb?.metadata?.namespace || !nb?.metadata?.name) {
      return;
    }

    this.sshNodePortSub.unsubscribe();

    const request = this.backend.getNotebookSshNodePort(
      nb.metadata.namespace,
      nb.metadata.name
    );

    this.sshNodePortSub = request.subscribe(
      nodePort => {
        this.prvSshNodePort = nodePort;
      },
      error => {
        // Silently handle error - SSH NodePort may not be available
        this.prvSshNodePort = null;
      }
    );
  }

  private generatePodDefaults(nb: NotebookRawObject) {
    const configurations = [];

    this.pollSub.unsubscribe();

    const request = this.backend.getPodDefaults(nb.metadata.namespace);

    this.pollSub = this.poller.exponential(request).subscribe(podDefaults => {
      for (const pd of podDefaults) {
        for (const label in nb.metadata.labels) {
          if (label === Object.keys(pd.spec.selector.matchLabels)[0]) {
            configurations.push(this.getPodDefaultConfiguration(pd));
          }
        }
      }
      this.configurations = configurations;
      this.podDefaults = podDefaults;
      this.generatePodEnv(this.pod);
    });
  }

  private getPodDefaultConfiguration(pd: PodDefault) {
    const configuration = {
      name: pd.metadata.name,
      Description: pd.spec.desc,
      Selector: pd.spec.selector,
      Env: pd.spec.env,
      Volumes: pd.spec.volumes,
      VolumeMounts: pd.spec.volumeMounts,
      ServiceAccountName: pd.spec.serviceAccountName,
    };

    return configuration;
  }

  private generateVolGroups(nb: NotebookRawObject): VolumesGroup[] {
    const volGroups: VolumesGroup[] = [];
    for (const volume of nb?.spec?.template?.spec?.volumes) {
      const groupName = this.classifyVolume(volume);
      const namespace = nb.metadata.namespace;
      const volumeItem = this.getVolumeItem(volume, groupName, namespace);

      const index = volGroups.findIndex(group => group.name === groupName);
      if (index < 0) {
        if (groupName === PVCS) {
          volGroups.unshift(this.newVolGroup(volumeItem, groupName));
        } else {
          volGroups.push(this.newVolGroup(volumeItem, groupName));
        }
      } else {
        volGroups[index].array.push(volumeItem);
      }
    }
    return volGroups;
  }

  private classifyVolume(volume: V1Volume): string {
    if (volume?.persistentVolumeClaim) {
      return PVCS;
    } else if (volume?.emptyDir?.medium === 'Memory') {
      return MEMORY_VOLS;
    } else if (volume?.emptyDir) {
      return EPHEMERALS;
    } else if (volume?.configMap) {
      return CONFIG_MAPS;
    } else if (volume?.secret) {
      return SECRETS;
    }
    return OTHER_VOLS;
  }

  private newVolGroup(
    volItem: UrlItem | ChipDescriptor,
    groupName: string,
  ): VolumesGroup {
    const group: VolumesGroup = {
      name: groupName,
      array: [volItem],
    };
    if (groupName === PVCS) {
      group.info =
        'A PersistentVolumeClaim (PVC) is a request for storage by a user and it is similar to a Pod. Pods consume node resources and PVCs consume PV resources. Pods can request specific levels of resources (CPU and Memory). Claims can request specific size and access modes (e.g., they can be mounted ReadWriteOnce, ReadOnlyMany or ReadWriteMany).'; // eslint-disable-line
      group.url =
        'https://kubernetes.io/docs/concepts/storage/persistent-volumes/';
    } else if (groupName === MEMORY_VOLS) {
      group.info =
        "A memory-backed volume is a special kind of emptyDir volume which is mounted as a tmpfs (RAM-backed filesystem) volume. While tmpfs is very fast, be aware that unlike disks, tmpfs is cleared on node reboot and any files you write count against your container's memory limit."; // eslint-disable-line
      group.url =
        'https://kubernetes.io/docs/concepts/storage/volumes/#emptydir';
    } else if (groupName === EPHEMERALS) {
      group.info =
        'An ephemeral volume is Kubernetes emptyDir. An emptyDir volume is first created when a Pod is assigned to a node, and exists as long as that Pod is running on that node. As the name says, the emptyDir volume is initially empty. All containers in the Pod can read and write the same files in the emptyDir volume, though that volume can be mounted at the same or different paths in each container. When a Pod is removed from a node for any reason, the data in the emptyDir is deleted permanently.'; // eslint-disable-line
      group.url =
        'https://kubernetes.io/docs/concepts/storage/volumes/#emptydir';
    } else if (groupName === CONFIG_MAPS) {
      group.info =
        'A ConfigMap provides a way to inject configuration data into pods. The data stored in a ConfigMap can be referenced in a volume of type configMap and then consumed by containerized applications running in a pod.'; // eslint-disable-line
      group.url =
        'https://kubernetes.io/docs/concepts/storage/volumes/#configmap';
    } else if (groupName === SECRETS) {
      group.info =
        'A secret volume is used to pass sensitive information, such as passwords, to Pods. You can store secrets in the Kubernetes API and mount them as files for use by pods without coupling to Kubernetes directly. Secret volumes are backed by tmpfs (a RAM-backed filesystem) so they are never written to non-volatile storage.'; // eslint-disable-line
      group.url = 'https://kubernetes.io/docs/concepts/storage/volumes/#secret';
    } else if (groupName === OTHER_VOLS) {
      group.url = 'https://kubernetes.io/docs/concepts/storage/volumes';
    }
    return group;
  }

  private getVolumeItem(
    vol: V1Volume,
    groupName: string,
    ns: string,
  ): UrlItem | ChipDescriptor {
    if (groupName === PVCS) {
      return {
        name: vol.name,
        url: `/volumes/volume/details/${ns}/${vol.name}`,
      };
    }
    return {
      value: vol.name,
      color: 'primary',
    };
  }

  private generateEnv(nb: NotebookRawObject): ChipDescriptor[] {
    for (const cn of nb.spec.template.spec.containers) {
      if (cn.name !== nb.metadata.name) {
        continue;
      }

      if (!cn.env) {
        return null;
      }

      const envGroup: EnvironmentVariablesGroup = {
        name: 'Notebook CR',
        chipsList: [],
      };
      for (const envVar of cn.env) {
        envGroup.chipsList.push(this.getEnvVarChip(envVar));
      }

      this.addEnvGroup(envGroup);

      return envGroup.chipsList;
    }
  }

  private generatePodEnv(pod: V1Pod): ChipDescriptor[] {
    if (!this.podDefaults || !pod) {
      return;
    }
    for (const cn of pod.spec.containers) {
      if (cn.name !== pod.metadata.labels['notebook-name']) {
        continue;
      }

      if (!cn.env) {
        return null;
      }

      this.initializePodEnvGroups();

      envLoop: for (const envVar of cn.env) {
        // Skip EnvVars set from Notebook CR
        const envChip = `${envVar.name}: ${envVar.value}`;
        if (this.notebookEnv?.map(env => env.value).includes(envChip)) {
          continue envLoop;
        }

        // Classify Enviornment Variable according to Configurations - Pod Defaults
        for (const envGroup of this.envGroups) {
          if (!envGroup.configuration) {
            continue;
          }

          if (this.envExistsInGroupConfiguration(envVar, envGroup)) {
            this.addEnvToGroup(envVar, envGroup);
            continue envLoop;
          }
        }

        const other = this.envGroups.filter(
          envGroup => envGroup.name === 'Other',
        )[0];
        this.addEnvToGroup(envVar, other);
      }

      this.cleanEmptyEnvGroups();
    }
  }

  private initializePodEnvGroups() {
    // Initialize Environment Variables Groups
    for (const configuration of this.podDefaults) {
      const envVarGroup: EnvironmentVariablesGroup = {
        name: `${configuration.metadata.name} (Configuration)`,
        configuration,
        chipsList: [],
      };
      this.addEnvGroup(envVarGroup);
    }
    const envGroup: EnvironmentVariablesGroup = {
      name: 'Other',
      chipsList: [],
    };
    this.addEnvGroup(envGroup);
  }

  private removePodEnvGroups() {
    if (!this.podDefaults) {
      return;
    }
    for (const group of this.envGroups) {
      const name = group.name.replace(' (Configuration)', '');
      if (
        this.podDefaults.map(pd => pd.metadata.name).includes(name) ||
        name === 'Other'
      ) {
        this.envGroups = this.envGroups.filter(
          envGroup => envGroup.name !== group.name,
        );
      }
    }
  }

  private cleanEmptyEnvGroups() {
    this.envGroups = this.envGroups.filter(
      envGroup => envGroup.chipsList.length !== 0,
    );
  }

  private addEnvGroup(envGroup: EnvironmentVariablesGroup) {
    const index = this.envGroups.findIndex(
      group => group.name === envGroup.name,
    );
    if (index < 0) {
      this.envGroups.push(envGroup);
    } else if (isEqual(this.envGroups[index], envGroup)) {
      return;
    } else {
      this.envGroups[index] = envGroup;
    }
  }

  private envExistsInGroupConfiguration(
    envVar: V1EnvVar,
    envGroup: EnvironmentVariablesGroup,
  ): boolean {
    if (
      envGroup.configuration?.spec?.env
        ?.map(env => env.name)
        .includes(envVar.name)
    ) {
      return true;
    }
    return false;
  }

  private addEnvToGroup(env: V1EnvVar, group: EnvironmentVariablesGroup) {
    const envValue = this.getEnvVarChip(env);
    if (group.chipsList.map(chip => chip.value).includes(envValue.value)) {
      return;
    } else {
      group.chipsList.push(envValue);
      return;
    }
  }

  private getEnvVarChip(envVar: V1EnvVar): ChipDescriptor {
    if (envVar.value) {
      return {
        value: `${envVar.name}: ${envVar.value}`,
        color: 'primary',
      };
    } else if (envVar.valueFrom) {
      const path = envVar.valueFrom.fieldRef.fieldPath;
      const envValue = get(this.pod, path);
      return {
        value: `${envVar.name}: ${envValue}`,
        color: 'primary',
      };
    }
  }
}
