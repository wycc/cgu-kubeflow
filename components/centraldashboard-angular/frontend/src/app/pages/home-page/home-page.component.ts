import { Component, OnInit } from '@angular/core';
import { EnvironmentService } from 'src/app/services/environment.service';
import { CDBBackendService } from 'src/app/services/backend.service';
import { DashboardLinks, Link } from 'src/app/types/dashboard-links';

@Component({
  selector: 'app-home-page',
  templateUrl: './home-page.component.html',
  styleUrls: ['./home-page.component.scss'],
})
export class HomePageComponent implements OnInit {
  public quickLinks: Link[] = [];
  public documentationItems: Link[] = [];
  public announcements: any[] = [];

  constructor(
    private env: EnvironmentService,
    private backend: CDBBackendService,
  ) {}

  ngOnInit() {
    this.env.dashboardLinks.subscribe((links: DashboardLinks) => {
      this.quickLinks = links.quickLinks || [];
      this.documentationItems = links.documentationItems || [];
    });

    this.backend.getAnnouncements().subscribe(
      (response: any) => {
        this.announcements = response.announcements || [];
      },
      (error) => {
        console.log('Announcements not available');
        this.announcements = [];
      },
    );
  }

  announcementTypeClass(type: string): string {
    const typeMap: { [key: string]: string } = {
      info: 'announcement-info',
      warning: 'announcement-warning',
      error: 'announcement-error',
      success: 'announcement-success',
    };
    return typeMap[type] || 'announcement-info';
  }
}
