// Progressive enhancement only: every admin action also works as a plain form
// post when JavaScript is unavailable.
(function () {
  "use strict";

  // Confirmation for destructive forms, without inline event handlers so the
  // strict Content-Security-Policy stays intact.
  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    var message = form.getAttribute("data-confirm");
    if (message && !window.confirm(message)) {
      event.preventDefault();
    }
  });

  // Keep the CSRF header in sync if HTMX is present.
  document.addEventListener("htmx:configRequest", function (event) {
    var match = document.cookie.match(/(?:^|;\s*)qrp_csrf=([^;]+)/);
    if (match) {
      event.detail.headers["X-CSRF-Token"] = decodeURIComponent(match[1]);
    }
  });
})();
