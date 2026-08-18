// Progressive enhancement for the share control. The button ships hidden, so a
// browser without JavaScript simply never sees an action it cannot perform;
// everything else on the profile keeps working.
(function () {
  "use strict";

  var button = document.getElementById("share-button");
  if (!button) return;

  var url = button.getAttribute("data-share-url") || window.location.href;
  var title = button.getAttribute("data-share-title") || document.title;
  var status = document.getElementById("share-status");

  var canShare = typeof navigator.share === "function";
  var canCopy = !!(navigator.clipboard && navigator.clipboard.writeText);

  // Nothing useful to offer: leave the button hidden.
  if (!canShare && !canCopy) return;
  button.hidden = false;

  function say(message) {
    if (!status) return;
    status.textContent = message;
    window.setTimeout(function () {
      status.textContent = "";
    }, 2500);
  }

  button.addEventListener("click", function () {
    if (canShare) {
      navigator.share({ title: title, url: url }).catch(function (err) {
        // A user who dismisses the share sheet is not an error worth showing.
        if (err && err.name === "AbortError") return;
        if (canCopy) copy();
      });
      return;
    }
    copy();
  });

  function copy() {
    navigator.clipboard.writeText(url).then(
      function () {
        say("Link copied");
      },
      function () {
        say("Could not copy the link");
      }
    );
  }
})();
