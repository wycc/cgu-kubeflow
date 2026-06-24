import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { IndexDefaultComponent } from './index-default.component';
import { KubeflowModule } from 'kubeflow';
import { ServerTypeComponent } from './server-type/server-type.component';
import { SshToggleComponent } from './ssh-toggle/ssh-toggle.component';

import { FormsModule } from '@angular/forms';
import { MatDialogModule } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';

/* Lance begin 20240906 */
import { AddPostConfirmDialogComponent } from './add-post-confirm-dialog/add-post-confirm-dialog.component';
import { AddPostDialogComponent } from './add-post-dialog/add-post-dialog.component';
import { AfterPostNotifyComponent } from './after-post-notify/after-post-notify.component';
import { DialogSharingModule } from './dialog-sharing/dialog-sharing.module';
/* Lance end 20240906 */

@NgModule({
  declarations: [
    IndexDefaultComponent,
    ServerTypeComponent,
    SshToggleComponent,
    AddPostConfirmDialogComponent,
    AddPostDialogComponent,
    AfterPostNotifyComponent,
  ],
  imports: [
    CommonModule,
    KubeflowModule,
    MatDialogModule,
    FormsModule,
    MatFormFieldModule,
    MatSlideToggleModule,
    DialogSharingModule,
  ],
  exports: [IndexDefaultComponent, ServerTypeComponent],
})
export class IndexDefaultModule {}
