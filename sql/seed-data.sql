INSERT INTO statuses (status, class_name, created_at, updated_at, is_error) VALUES
('Operational', 'text-operational', date('now'), date('now'), false),
('Degraded Performance', 'text-degraded-performance', date('now'), date('now'), true),
('Partial Outage', 'text-partial-outage', date('now'), date('now'), true),
('Major Outage', 'text-major-outage', date('now'), date('now'), true),
('Maintenance', 'text-under-maintenance', date('now'), date('now'), true)
;

INSERT INTO services (service_name, created_at, updated_at) VALUES
('Admin', date('now'), date('now')),
('Checkout', date('now'), date('now')),
('Reports and Dashboards', date('now'), date('now')),
('Storefront', date('now'), date('now')),
('API & Mobile', date('now'), date('now')),
('Third party services', date('now'), date('now')),
('Support', date('now'), date('now')),
('Point of Sale', date('now'), date('now')),
('Oxygen', date('now'), date('now'))
;

