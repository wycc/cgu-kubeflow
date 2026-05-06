import { Component, EventEmitter } from '@angular/core';
import { MatSlideToggleChange } from '@angular/material/slide-toggle';
import { STATUS_TYPE, ActionEvent } from 'kubeflow';
import { TableColumnComponent } from 'kubeflow/lib/resource-table/component-value/component-value.component';
import { NotebookProcessedObject } from 'src/app/types';

const SSH_SERVICE_LABEL = 'cgu.kubeflow.org/sshservice';

@Component({
  selector: 'app-ssh-toggle',
  templateUrl: './ssh-toggle.component.html',
  styleUrls: ['./ssh-toggle.component.scss'],
})
export class SshToggleComponent implements TableColumnComponent {
  element: NotebookProcessedObject;
  emitter = new EventEmitter<ActionEvent>();

  get checked(): boolean {
    return this.element?.metadata?.labels?.[SSH_SERVICE_LABEL] === 'true';
  }

  get disabled(): boolean {
    return (
      this.element?.sshAction === STATUS_TYPE.WAITING ||
      this.element?.sshAction === STATUS_TYPE.UNAVAILABLE
    );
  }

  onChange(event: MatSlideToggleChange) {
    this.emitter.emit(new ActionEvent('toggle-ssh', this.element));
  }
}
