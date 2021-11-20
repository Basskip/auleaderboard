document.addEventListener("DOMContentLoaded", updateTime);

const formatter = new Intl.RelativeTimeFormat(undefined, {
  numeric: "auto",
});

const DIVISIONS = [
  { amount: 60, name: "seconds" },
  { amount: 60, name: "minutes" },
  { amount: 24, name: "hours" },
  { amount: 7, name: "days" },
  { amount: 4.34524, name: "weeks" },
  { amount: 12, name: "months" },
  { amount: Number.POSITIVE_INFINITY, name: "years" },
];

function formatTimeAgo(date) {
  let duration = (date * 1000 - new Date().getTime()) / 1000;
  console.log(duration);

  for (let i = 0; i <= DIVISIONS.length; i++) {
    const division = DIVISIONS[i];
    if (Math.abs(duration) < division.amount) {
      return formatter.format(Math.round(duration), division.name);
    }
    duration /= division.amount;
  }
}

function updateTime() {
  const timearea = document.querySelector("#last-update");
  const timevalue = document.querySelector("#timestamp");
  timearea.textContent =
    "Last updated: " + formatTimeAgo(parseInt(timevalue.textContent));
}
