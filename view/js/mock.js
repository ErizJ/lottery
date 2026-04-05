/**
 * mock.js — 前端 Mock 拦截器
 * 始终被 lottery.html 加载；仅当 URL 含 ?mock=1 时激活。
 * 拦截 jQuery $.ajax，模拟后端两个接口的返回数据。
 */
;(function ($) {
  'use strict';

  /* URL 不含 ?mock=1 时直接退出，不影响真实后端 */
  if (new URLSearchParams(location.search).get('mock') !== '1') return;

  /* 注入 Mock 标记角标 */
  var badge = document.createElement('div');
  badge.className = 'mock-badge';
  badge.textContent = 'MOCK MODE';
  document.body.appendChild(badge);

  /* ── Mock 奖品数据（与 db/bak.sql 一致） ── */
  var mockGifts = [
    { ID: 2,  Name: '篮球',  Description: '价值 ¥100',  Picture: 'img/ball.jpeg',   Price: 100,  Count: 0 },
    { ID: 3,  Name: '水杯',  Description: '价值 ¥80',   Picture: 'img/cup.jpeg',    Price: 80,   Count: 0 },
    { ID: 4,  Name: '电脑',  Description: '价值 ¥6000', Picture: 'img/laptop.jpeg', Price: 6000, Count: 0 },
    { ID: 5,  Name: '平板',  Description: '价值 ¥4000', Picture: 'img/pad.jpg',     Price: 4000, Count: 0 },
    { ID: 6,  Name: '手机',  Description: '价值 ¥5000', Picture: 'img/phone.jpeg',  Price: 5000, Count: 0 },
    { ID: 7,  Name: '锅',    Description: '价值 ¥120',  Picture: 'img/pot.jpeg',    Price: 120,  Count: 0 },
    { ID: 8,  Name: '茶叶',  Description: '价值 ¥90',   Picture: 'img/tea.jpeg',    Price: 90,   Count: 0 },
    { ID: 9,  Name: '无人机',Description: '价值 ¥400',  Picture: 'img/uav.jpeg',    Price: 400,  Count: 0 },
    { ID: 10, Name: '酒',    Description: '价值 ¥160',  Picture: 'img/wine.jpeg',   Price: 160,  Count: 0 }
  ];

  /* ── Mock 库存（可按权重控制稀有度） ── */
  var mockInventory = {
    2: 8, 3: 8, 4: 2, 5: 3, 6: 4, 7: 8, 8: 8, 9: 1, 10: 4
  };

  /* ── 按剩余库存加权随机抽奖 ── */
  function mockLottery() {
    var ids = [], weights = [], total = 0;
    for (var id in mockInventory) {
      if (mockInventory[id] > 0) {
        ids.push(parseInt(id));
        weights.push(mockInventory[id]);
        total += mockInventory[id];
      }
    }
    if (ids.length === 0) return '0';

    var rand = Math.random() * total;
    var cum  = 0;
    for (var i = 0; i < ids.length; i++) {
      cum += weights[i];
      if (rand < cum) {
        mockInventory[ids[i]]--;
        return String(ids[i]);
      }
    }
    return String(ids[ids.length - 1]);
  }

  /* ── 拦截 $.ajax ── */
  var _origAjax = $.ajax.bind($);
  $.ajax = function (options) {
    if (!options || typeof options.url !== 'string') {
      return _origAjax.apply(this, arguments);
    }

    var url = options.url;

    /* GET api/v1/gifts */
    if (url.indexOf('api/v1/gifts') !== -1) {
      console.log('[Mock] GET api/v1/gifts');
      setTimeout(function () {
        if (typeof options.success === 'function') {
          options.success({
            status: 200,
            msg: 'OK',
            total: mockGifts.length,
            data: mockGifts
          });
        }
      }, 200);
      return { fail: function () { return this; }, done: function () { return this; } };
    }

    /* GET api/v1/lucky */
    if (url.indexOf('api/v1/lucky') !== -1) {
      console.log('[Mock] GET api/v1/lucky');
      setTimeout(function () {
        var result = mockLottery();
        console.log('[Mock] lucky result =', result);
        if (typeof options.success === 'function') {
          options.success(result);
        }
      }, 120);
      return { fail: function () { return this; }, done: function () { return this; } };
    }

    /* 其余请求正常走 */
    return _origAjax.apply(this, arguments);
  };

  console.log(
    '%c[Mock] 已启动 — 库存：' + JSON.stringify(mockInventory),
    'color:#67E8F9;font-weight:bold'
  );

})(jQuery);
