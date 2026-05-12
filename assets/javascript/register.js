var isSafari = window.safari !== undefined

if ('serviceWorker' in navigator) {
  if (isSafari) {
    navigator.serviceWorker.getRegistrations().then(function (registrations) {
      for (let registration of registrations) {
        registration.unregister()
      }
    })
  } else {
    navigator.serviceWorker.register("/sw.js").then(registration => {
      console.log('service worker registered:', registration);
    }).catch(error => {
      console.log('service worker error:', error)
    })
  }
}