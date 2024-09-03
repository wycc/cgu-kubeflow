import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { IndexRokComponent } from './index-rok.component';
import { KubeflowModule } from 'kubeflow';

import { I } from '@angular/cdk/keycodes';
import { IndexDefaultModule2 } from '../../index2/index-default2/index-default2.module';

@NgModule({
  declarations: [IndexRokComponent],
  imports: [CommonModule, KubeflowModule,IndexDefaultModule2],
  exports: [IndexRokComponent],
})
export class IndexRokModule {}
